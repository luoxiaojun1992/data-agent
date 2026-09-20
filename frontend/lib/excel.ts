// Excel 解析工具：用 read-excel-file（MIT）把 .xlsx 解析为纯文本。
// 仅支持 .xlsx（D5 已收敛，不支持 .xls）；Excel 只解析为纯文本、不提取图片
// （SPEC-096 D4/D5）。逐 sheet 拼接单元格文本（sheet 间用空行分隔，单元格间
// 用制表符分隔，行间用换行分隔）。

export interface ExcelParseResult {
  text: string; // 全部 sheet 拼接的纯文本（无图片）
}

/**
 * 解析 .xlsx：逐 sheet 拼接单元格文本（纯文本，无图片）。
 * 动态 import `read-excel-file/browser`，仅在浏览器上传时加载，避免 SSR 阶段
 * 引入浏览器专用模块（与 parsePdf 动态 import unpdf 同套路）。
 */
export async function parseExcel(file: File): Promise<ExcelParseResult> {
  const { default: readXlsxFile } = await import('read-excel-file/browser');
  const sheets = await readXlsxFile(file);
  const parts: string[] = [];
  for (const sheet of sheets) {
    const lines = sheet.data.map((row) =>
      row.map((cell) => excelCellToString(cell)).join('\t'),
    );
    parts.push(lines.join('\n'));
  }
  return { text: parts.join('\n\n') };
}

// 单元格值序列化：数字→十进制字符串、布尔→true/false、日期→ISO 8601、
// null/undefined→空串、其余（字符串等）原样（SPEC-096 D4，实现阶段细化）。
function excelCellToString(cell: unknown): string {
  if (cell === null || cell === undefined) return '';
  if (cell instanceof Date) return cell.toISOString();
  if (typeof cell === 'number') return String(cell);
  if (typeof cell === 'boolean') return cell ? 'true' : 'false';
  return String(cell);
}

/** 根据文件名扩展名判断是否为 Excel（仅 .xlsx，D5 不支持 .xls）。 */
export function isExcelFile(fileName: string): boolean {
  return fileName.toLowerCase().endsWith('.xlsx');
}
