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
