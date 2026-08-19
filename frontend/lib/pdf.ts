// PDF 解析工具：在浏览器端用 pdf.js 解析 PDF，提取纯文本 + 嵌入的配图（图片对象）。
// 仅在客户端（浏览器）运行，通过 dynamic import 加载 pdfjs-dist。

export interface PdfImage {
  dataUrl: string; // base64 data URL
  mimeType: string; // image/jpeg / image/png
  ext: string;      // jpg / png
}

export interface PdfParseResult {
  text: string; // 合并所有页的纯文本
  images: PdfImage[]; // 从 PDF 中提取出的嵌入配图（每张配图一个文件）
}

// pdfjs 的 worker 配置：worker 文件作为静态资源放在 public/ 下，
// 避免被 webpack 当作模块用 Terser 压缩（.mjs 的 import/export 会报错）。
// 懒加载，避免在服务端渲染时引入 pdfjs-dist。
async function loadPdfjs() {
  const pdfjs = await import('pdfjs-dist');
  if (!pdfjs.GlobalWorkerOptions.workerSrc) {
    pdfjs.GlobalWorkerOptions.workerSrc = '/pdf.worker.min.mjs';
  }
  return pdfjs;
}

/**
 * 解析 PDF 文件：返回合并的纯文本 + 从 PDF 中提取出的嵌入配图。
 * 配图通过操作符列表（paintImageXObject）定位图片对象，而非整页渲染，
 * 因此每一张嵌入的图片会作为一个独立的图片文件返回。
 */
export async function parsePdf(file: File): Promise<PdfParseResult> {
  const pdfjs = await loadPdfjs();
  const arrayBuffer = await file.arrayBuffer();
  const pdf = await pdfjs.getDocument({ data: arrayBuffer }).promise;

  let text = '';
  const images: PdfImage[] = [];

  for (let pageNum = 1; pageNum <= pdf.numPages; pageNum++) {
    const page = await pdf.getPage(pageNum);

    // 1. 提取文本
    const textContent = await page.getTextContent();
    const pageText = (textContent.items as { str?: string }[])
      .map((item) => item.str || '')
      .join(' ');
    text += pageText.trim() + '\n';

    // 2. 提取本页嵌入的配图（图片对象，非整页渲染）
    const pageImages = await extractPageImages(page, pdfjs);
    images.push(...pageImages);
  }

  return { text, images };
}

// 提取单页里所有嵌入的图片对象（去重）。
async function extractPageImages(page: any, pdfjs: any): Promise<PdfImage[]> {
  const opList = await page.getOperatorList();
  const xobjOps = [pdfjs.OPS.paintImageXObject, pdfjs.OPS.paintImageXObjectRepeat];
  console.log(`[pdf] page operator list: ${opList.fnArray.length} ops`);

  const seen = new Set<string>();
  const candidates: string[] = [];
  for (let i = 0; i < opList.fnArray.length; i++) {
    if (!xobjOps.includes(opList.fnArray[i])) continue;
    const imgName = opList.argsArray[i]?.[0];
    if (!imgName || typeof imgName !== 'string' || seen.has(imgName)) continue;
    seen.add(imgName);
    candidates.push(imgName);
  }
  console.log(`[pdf] image candidates: ${candidates.length} (${candidates.join(',')})`);

  const images: PdfImage[] = [];
  const failures: string[] = [];
  for (const imgName of candidates) {
    try {
      const img: any = await new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error(`page.objs.get(${imgName}) timeout 10s`)), 10000);
        page.objs.get(imgName, (obj: any) => {
          clearTimeout(timer);
          resolve(obj);
        });
      });
      if (!img) { failures.push(`${imgName}=null`); continue; }
      if (!img.data) { failures.push(`${imgName}=no data`); continue; }
      console.log(`[pdf] image ${imgName}: ${img.width}x${img.height}, kind=${img.kind}, data=${img.data.byteLength || img.data.length}B`);
      const out = imageObjectToDataUrl(img, pdfjs);
      if (out) {
        images.push(out);
        console.log(`[pdf] image ${imgName}: converted to ${out.ext} (${out.dataUrl.length} chars dataUrl)`);
      } else {
        failures.push(`${imgName}=null conversion`);
      }
    } catch (e: any) {
      console.error(`[pdf] image ${imgName} error:`, e);
      failures.push(`${imgName}=${e?.message || e}`);
    }
  }
  if (candidates.length > 0 && images.length === 0) {
    throw new Error(
      `PDF 配图提取全部失败：找到 ${candidates.length} 张候选配图但一张都没成功。` +
      `失败明细：${failures.join('; ')}（浏览器控制台 [pdf] 日志有更多细节）`,
    );
  }
  if (failures.length > 0) {
    console.warn(`[pdf] 部分配图失败 (${failures.length}/${candidates.length}): ${failures.join('; ')}`);
  }
  return images;
}

// 把 pdf.js 的图片对象转成 data URL。
// 优先识别 JPEG/PNG 原始字节直接复用；否则当作解码后的像素数据，
// 用 canvas 重新编码为 PNG。
function imageObjectToDataUrl(img: any, pdfjs: any): PdfImage | null {
  const data = img.data instanceof Uint8Array ? img.data : new Uint8Array(img.data);
  if (data.length === 0) return null;

  // JPEG: FF D8
  if (data.length > 2 && data[0] === 0xff && data[1] === 0xd8) {
    return { dataUrl: uint8ToDataUrl(data, 'image/jpeg'), mimeType: 'image/jpeg', ext: 'jpg' };
  }
  // PNG: 89 50 4E 47
  if (data.length > 4 && data[0] === 0x89 && data[1] === 0x50 && data[2] === 0x4e && data[3] === 0x47) {
    return { dataUrl: uint8ToDataUrl(data, 'image/png'), mimeType: 'image/png', ext: 'png' };
  }

  // 解码后的像素数据 → canvas 重新编码为 PNG
  const canvas = document.createElement('canvas');
  canvas.width = img.width;
  canvas.height = img.height;
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    throw new Error(`canvas 2d context unavailable (image ${img.width}x${img.height})`);
  }
  const imageData = ctx.createImageData(img.width, img.height);
  fillPixelData(imageData.data, data, img.width, img.height, img.kind, pdfjs.ImageKind);
  ctx.putImageData(imageData, 0, 0);
  const dataUrl = canvas.toDataURL('image/png');
  if (!dataUrl || dataUrl === 'data:,') {
    throw new Error(`canvas.toDataURL returned empty for image ${img.width}x${img.height} (kind=${img.kind})`);
  }
  return { dataUrl, mimeType: 'image/png', ext: 'png' };
}

// 把 pdf.js 的像素数据（RGBA/RGB/灰度 1bpp）填入 ImageData 的 RGBA 缓冲。
function fillPixelData(
  dest: Uint8ClampedArray,
  src: Uint8Array,
  w: number,
  h: number,
  kind: number,
  ImageKind: any,
) {
  if (kind === ImageKind.RGBA_32BPP) {
    dest.set(src.subarray(0, Math.min(src.length, w * h * 4)));
  } else if (kind === ImageKind.RGB_24BPP) {
    const n = Math.min(w * h, Math.floor(src.length / 3));
    for (let i = 0; i < n; i++) {
      dest[i * 4] = src[i * 3];
      dest[i * 4 + 1] = src[i * 3 + 1];
      dest[i * 4 + 2] = src[i * 3 + 2];
      dest[i * 4 + 3] = 255;
    }
  } else if (kind === ImageKind.GRAYSCALE_1BPP) {
    const rowBytes = (w + 7) >> 3;
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const byte = src[y * rowBytes + (x >> 3)];
        const bit = (byte >> (7 - (x & 7))) & 1;
        const v = bit ? 255 : 0;
        const idx = (y * w + x) * 4;
        dest[idx] = v;
        dest[idx + 1] = v;
        dest[idx + 2] = v;
        dest[idx + 3] = 255;
      }
    }
  }
}

// Uint8Array → base64 data URL（分块避免 spread 大数组栈溢出）。
function uint8ToDataUrl(data: Uint8Array, mimeType: string): string {
  let binary = '';
  const chunk = 0x4000;
  for (let i = 0; i < data.length; i += chunk) {
    binary += String.fromCharCode.apply(null, Array.from(data.subarray(i, i + chunk)));
  }
  return `data:${mimeType};base64,${btoa(binary)}`;
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
