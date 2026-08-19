// PDF 解析工具：在浏览器端用 pdf.js 解析 PDF，提取纯文本 + 每页图片。
// 仅在客户端（浏览器）运行，通过 dynamic import 加载 pdfjs-dist。

export interface PdfImage {
  dataUrl: string; // base64 data URL
  mimeType: string; // image/png
}

export interface PdfParseResult {
  text: string; // 合并所有页的纯文本
  images: PdfImage[]; // 每页一张图片
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
 * 解析 PDF 文件：返回合并的纯文本 + 每页渲染出的 PNG 图片（base64）。
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

    // 2. 渲染页面为 PNG 图片
    const viewport = page.getViewport({ scale: 2.0 });
    const canvas = document.createElement('canvas');
    canvas.width = Math.floor(viewport.width);
    canvas.height = Math.floor(viewport.height);
    const ctx = canvas.getContext('2d');
    if (!ctx) {
      throw new Error('canvas 2d context unavailable');
    }
    await page.render({ canvasContext: ctx, viewport }).promise;
    images.push({
      dataUrl: canvas.toDataURL('image/png'),
      mimeType: 'image/png',
    });
  }

  return { text, images };
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
