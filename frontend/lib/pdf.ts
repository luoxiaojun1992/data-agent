// PDF 解析工具：在浏览器端用 unpdf（unjs）解析 PDF，提取纯文本 + 嵌入的配图。
// unpdf 内置 serverless 构建的 PDF.js，提供跨运行时（Node/浏览器）稳定的
// extractText / extractImages API，无需手动 hack PDF.js 内部对象。

export interface PdfImage {
  dataUrl: string; // base64 data URL
  mimeType: string; // image/png
  ext: string;      // png
}

export interface PdfParseResult {
  text: string; // 合并所有页的纯文本
  images: PdfImage[]; // 从 PDF 中提取出的嵌入配图（每张配图一个文件）
}

// unpdf 的 extractImages 返回的原始图片对象。
interface RawImage {
  data: Uint8ClampedArray; // 解码后的像素（1/3/4 通道）
  width: number;
  height: number;
  channels: 1 | 3 | 4; // 1=灰度, 3=RGB, 4=RGBA
  key: string;         // 图片对象名，用于跨页去重
}

/**
 * 解析 PDF：合并的纯文本 + 提取出的嵌入配图。
 *
 * 注意：unpdf 的 extractImages 底层用的是 PDF.js 的 page.objs.get。
 * 在浏览器里，只调用 extractImages 拿到的图片对象缺少 data 字段
 * （worker 还没跑完图片解码）。必须先渲染整页触发 worker 解码，
 * 然后再 extractImages 才能拿到带 data 的对象（Node 环境 PDF.js
 * 同步解码所以不需要这一步）。
 */
export async function parsePdf(file: File): Promise<PdfParseResult> {
  const { getDocumentProxy, extractText, extractImages, renderPageAsImage } = await import('unpdf');
  const buffer = new Uint8Array(await file.arrayBuffer());
  const pdf = await getDocumentProxy(buffer);

  // 1. 提取合并文本
  const { text } = await extractText(pdf, { mergePages: true });

  // 2. 强制渲染每页（触发 worker 异步解码图片），丢弃输出。
  //    小尺寸（scale=0.1）足够触发解码、又快、内存少。
  for (let pageNum = 1; pageNum <= pdf.numPages; pageNum++) {
    await renderPageAsImage(pdf, pageNum, { scale: 0.1 });
  }

  // 3. 现在图片对象已解码，逐页提取嵌入配图
  const seen = new Set<string>();
  const images: PdfImage[] = [];
  for (let pageNum = 1; pageNum <= pdf.numPages; pageNum++) {
    const imgs = await extractImages(pdf, pageNum);
    for (const img of imgs) {
      if (seen.has(img.key)) continue;
      seen.add(img.key);
      const dataUrl = pixelToPngDataUrl(img);
      if (dataUrl) {
        images.push({ dataUrl, mimeType: 'image/png', ext: 'png' });
      }
    }
  }

  return { text, images };
}

// 把 unpdf 提取出的像素数据（1/3/4 通道）转成 PNG data URL。
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
