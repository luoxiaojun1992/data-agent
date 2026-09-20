// 图片附件共享工具：chat 与 agent 任务创建共用（base64 上传 + 预览）。

// Image attachment pending send: base64 for the wire, dataUrl for preview.
export interface Attachment {
  name: string;
  mimeType: string;
  base64: string;
  dataUrl: string;
}

// Image limits (mirrors backend domainchat): at most 5 images, 2MiB each.
export const MAX_ATTACHMENT_IMAGES = 5;
export const MAX_ATTACHMENT_IMAGE_BYTES = 2 * 1024 * 1024;

// PDF attachment limits (SPEC-077): 20MiB file size before parsing, and the
// merged text limit (user prompt + PDF parsed text, UTF-8 bytes) is 100KB.
export const MAX_PDF_BYTES = 20 * 1024 * 1024;
// Excel attachment limits (SPEC-096): 20MiB file size before parsing, text-only.
export const MAX_EXCEL_BYTES = 20 * 1024 * 1024;
export const MAX_CHAT_TEXT_BYTES = 100 * 1024;

// PDF attachment pending send: only name + parsed text (text never rendered).
export interface PdfAttachment {
  name: string;
  text: string;
}

// Excel attachment pending send: only name + parsed text (text never rendered).
export interface ExcelAttachment {
  name: string;
  text: string;
}

// 附件标签剥离结果：干净文本 + PDF/Excel 文件名（仅 name，解析文字绝不展示）。
export interface StrippedAttachment {
  text: string;
  pdfs: { name: string }[];
  excels: { name: string }[];
}

// 剥离后端前置的 PDF/Excel 标签块（[PDF:name]…[/PDF:name] /
// [Excel:name]…[/Excel:name]），返回干净文本 + 附件文件名（SPEC-077 §5.3 /
// SPEC-096 D3）。仅用于 user 文本事件的渲染；解析文字绝不展示给用户。
export function stripAttachmentBlocks(content: string): StrippedAttachment {
  const pdfRe = /\[PDF:([^\]]+)\][\s\S]*?\[\/PDF:[^\]]+\]/g;
  const excelRe = /\[Excel:([^\]]+)\][\s\S]*?\[\/Excel:[^\]]+\]/g;
  const pdfs: { name: string }[] = [];
  const excels: { name: string }[] = [];
  let text = content.replace(pdfRe, (_full, name: string) => {
    pdfs.push({ name });
    return '';
  });
  text = text.replace(excelRe, (_full, name: string) => {
    excels.push({ name });
    return '';
  });
  return { text: text.trim(), pdfs, excels };
}

// Read an image File into a pending attachment (base64 + preview data URL).
export function fileToAttachment(file: File): Promise<Attachment> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = String(reader.result || '');
      const base64 = dataUrl.split(',')[1] || '';
      resolve({
        name: file.name || 'image',
        mimeType: file.type || 'image/png',
        base64,
        dataUrl,
      });
    };
    reader.onerror = () => reject(new Error('读取图片失败'));
    reader.readAsDataURL(file);
  });
}
