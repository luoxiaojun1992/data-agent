// PDF 解析工具：在浏览器端用 unpdf（unjs）解析 PDF，提取纯文本 + 嵌入的配图。
// 文本用 unpdf 的 extractText；嵌入配图则直接从 PDF.js 的页面对象缓存中读取，
// 因为 unpdf 的 extractImages 在浏览器里拿不到图片数据（见下方 parsePdf 注释）。

export interface PdfImage {
  dataUrl: string; // base64 data URL
  mimeType: string; // image/png
  ext: string;      // png
}

export interface PdfParseResult {
  text: string; // 合并所有页的纯文本
  images: PdfImage[]; // 从 PDF 中提取出的嵌入配图（每张配图一个文件）
}

// PDF.js 解码后的图片对象（page.objs 缓存中拿到）。
// 浏览器里像素存在 bitmap 字段，但类型不定：
//   - FlateDecode（PNG 等无损）→ ImageBitmap
//   - DCTDecode（JPEG）→ VideoFrame（WebCodecs）
// 旧环境无 ImageBitmap/VideoFrame 时像素在 data 字段（Uint8ClampedArray）。
interface DecodedImage {
  bitmap?: any; // ImageBitmap | VideoFrame
  data?: Uint8ClampedArray;
  width: number;
  height: number;
  channels?: 1 | 3 | 4; // 仅 data 兜底路径使用：1=灰度, 3=RGB, 4=RGBA
}

// 原始像素数据（data 兜底路径使用）。
interface RawImage {
  data: Uint8ClampedArray;
  width: number;
  height: number;
  channels: 1 | 3 | 4;
  key: string;
}

/**
 * 解析 PDF：合并的纯文本 + 提取出的嵌入配图。
 *
 * 为什么不能直接用 unpdf 的 extractImages：
 *   unpdf 内置的 PDF.js（v6.x）在浏览器里会用 createImageBitmap 把图片解码成
 *   ImageBitmap，存在图片对象的 `bitmap` 字段上，`data` 字段是 undefined；
 *   而 unpdf 的 extractImages 写死 `if (!image.data) continue`，于是浏览器里
 *   一张图都提不出来（Node 里无 ImageBitmap，走 Uint8ClampedArray 兜底才正常）。
 *   因此这里自己走：getOperatorList 找 paintImageXObject → 渲染整页触发解码 →
 *   从 page.objs 缓存按 key 取出图片 → 画到 canvas 转 PNG。
 */
export async function parsePdf(file: File): Promise<PdfParseResult> {
  const { getDocumentProxy, extractText, renderPageAsImage, getResolvedPDFJS } = await import('unpdf');
  const buffer = new Uint8Array(await file.arrayBuffer());
  const pdf = await getDocumentProxy(buffer);
  const { OPS } = await getResolvedPDFJS();

  // 1. 提取合并文本
  const { text } = await extractText(pdf, { mergePages: true });

  // 2. 逐页提取嵌入配图
  const seen = new Set<string>();
  const images: PdfImage[] = [];
  for (let pageNum = 1; pageNum <= pdf.numPages; pageNum++) {
    const page = await pdf.getPage(pageNum);
    const opList = await page.getOperatorList();

    // 收集本页所有 paintImageXObject / paintImageXObjectRepeat 的图片 key。
    // g_ 前缀的 key 存在 commonObjs，其余在 objs。
    const keys: { key: string; common: boolean }[] = [];
    for (let i = 0; i < opList.fnArray.length; i++) {
      const fn = opList.fnArray[i];
      if (fn !== OPS.paintImageXObject && fn !== OPS.paintImageXObjectRepeat) continue;
      const key = String(opList.argsArray[i]?.[0] ?? '');
      if (!key || seen.has(key)) continue;
      keys.push({ key, common: key.startsWith('g_') });
    }
    if (keys.length === 0) continue;

    // 渲染整页（小尺寸即可，只用来触发 PDF.js 解码图片到 page.objs 缓存；
    // 图片本身仍按原始分辨率解码，与渲染 scale 无关），输出直接丢弃。
    await renderPageAsImage(pdf, pageNum, { scale: 0.1 });

    for (const { key, common } of keys) {
      seen.add(key);
      let img: DecodedImage | undefined;
      try {
        img = (common ? page.commonObjs : page.objs).get(key) as DecodedImage | undefined;
      } catch {
        continue; // 对象未解码，跳过
      }
      const dataUrl = await imageToPngDataUrl(img);
      if (dataUrl) {
        images.push({ dataUrl, mimeType: 'image/png', ext: 'png' });
      }
    }
  }

  return { text, images };
}

// 把 PDF.js 解码出的图片对象转成 PNG data URL。
// 优先走 bitmap（ImageBitmap / VideoFrame，浏览器主路径）；无 bitmap 时走 data（Uint8ClampedArray 兜底）。
async function imageToPngDataUrl(img: DecodedImage | undefined | null): Promise<string | null> {
  if (!img) return null;
  try {
    if (img.bitmap) {
      // PDF.js 在浏览器里可能把图片解码成 ImageBitmap（无损）或 VideoFrame（JPEG）。
      // 两者统一成 ImageBitmap 再画到 canvas 转 PNG。
      const bmp: any = img.bitmap;
      const VideoFrameCtor = (globalThis as any).VideoFrame;
      let bitmap: ImageBitmap;
      if (bmp instanceof ImageBitmap) {
        bitmap = bmp;
      } else if (VideoFrameCtor && bmp instanceof VideoFrameCtor) {
        bitmap = await createImageBitmap(bmp); // VideoFrame → ImageBitmap
      } else {
        return null;
      }
      try {
        const canvas = document.createElement('canvas');
        canvas.width = bitmap.width;
        canvas.height = bitmap.height;
        const ctx = canvas.getContext('2d');
        if (!ctx) return null;
        ctx.drawImage(bitmap, 0, 0);
        return canvas.toDataURL('image/png');
      } finally {
        if (bitmap !== bmp) bitmap.close(); // 释放新建的 ImageBitmap
      }
    }
    if (img.data && img.width && img.height) {
      const channels = (img.channels ?? (img.data.length / (img.width * img.height))) as 1 | 3 | 4;
      return pixelToPngDataUrl({ data: img.data, width: img.width, height: img.height, channels, key: '' });
    }
    return null;
  } catch (e) {
    console.error('[pdf] image → png error:', e);
    return null;
  }
}

// 把像素数据（1/3/4 通道）转成 PNG data URL（无 ImageBitmap 环境的兜底路径）。
function pixelToPngDataUrl(img: RawImage): string | null {
  try {
    const canvas = document.createElement('canvas');
    canvas.width = img.width;
    canvas.height = img.height;
    const ctx = canvas.getContext('2d');
    if (!ctx) return null;

    const imageData = ctx.createImageData(img.width, img.height);
    const src = img.data;
    const dst = imageData.data;

    if (img.channels === 4) {
      dst.set(src);
    } else if (img.channels === 3) {
      for (let i = 0, j = 0; i < src.length; i += 3, j += 4) {
        dst[j] = src[i];
        dst[j + 1] = src[i + 1];
        dst[j + 2] = src[i + 2];
        dst[j + 3] = 255;
      }
    } else if (img.channels === 1) {
      for (let i = 0, j = 0; i < src.length; i++, j += 4) {
        dst[j] = src[i];
        dst[j + 1] = src[i];
        dst[j + 2] = src[i];
        dst[j + 3] = 255;
      }
    } else {
      return null;
    }

    ctx.putImageData(imageData, 0, 0);
    return canvas.toDataURL('image/png');
  } catch (e) {
    console.error('[pdf] pixelToPngDataUrl error:', e);
    return null;
  }
}

// 支持的图片扩展名（用于文件类型判断）。
const IMAGE_EXTENSIONS = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp'];

/** 根据文件名扩展名判断是否为图片。 */
export function isImageFile(fileName: string): boolean {
  const ext = fileName.split('.').pop()?.toLowerCase() || '';
  return IMAGE_EXTENSIONS.includes(ext);
}

/** 根据文件名扩展名判断是否为 PDF。 */
export function isPdfFile(fileName: string): boolean {
  return fileName.toLowerCase().endsWith('.pdf');
}

/** 根据文件名扩展名判断是否为 txt。 */
export function isTxtFile(fileName: string): boolean {
  return fileName.toLowerCase().endsWith('.txt');
}

/** 支持的扩展名（需求：仅 txt、pdf、图片）。 */
export const SUPPORTED_EXTENSIONS = ['txt', 'pdf', ...IMAGE_EXTENSIONS];

/** 根据图片文件名推断 MIME 类型。 */
export function imageMimeType(fileName: string): string {
  const ext = fileName.split('.').pop()?.toLowerCase() || '';
  switch (ext) {
    case 'png': return 'image/png';
    case 'jpg':
    case 'jpeg': return 'image/jpeg';
    case 'gif': return 'image/gif';
    case 'webp': return 'image/webp';
    case 'bmp': return 'image/bmp';
    default: return 'image/png';
  }
}
