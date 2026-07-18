/**
 * SPEC-046: KB indexing fixture — creates docs and triggers indexing via API.
 */
const API_BASE = 'http://data-agent:8080/api/v1';

export interface IndexedDoc {
  id: string;
  title: string;
  chunks: string[];
}

/**
 * Create a KB doc and add chunks (simulating LLM chunking).
 * Returns the doc info for later verification.
 */
export async function createAndIndexDoc(
  request: any,
  token: string,
  title: string,
  content: string,
  chunkSize = 200
): Promise<IndexedDoc | null> {
  // 1. Create doc metadata
  const createRes = await request.post(`${API_BASE}/knowledge/docs`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: {
      title,
      file_name: `${title}.md`,
      file_type: 'markdown',
      size_bytes: content.length,
    },
  });
  if (!createRes.ok()) return null;
  const doc = await createRes.json();

  // 2. Split content into chunks
  const lines = content.split('\n');
  const chunks: string[] = [];
  let current = '';
  for (const line of lines) {
    if (current.length + line.length > chunkSize && current.length > 0) {
      chunks.push(current.trim());
      current = line;
    } else {
      current += (current ? '\n' : '') + line;
    }
  }
  if (current.trim()) chunks.push(current.trim());

  // 3. Add chunks
  const chunkRes = await request.post(`${API_BASE}/knowledge/docs/${doc.id}/chunks`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: { chunks },
  });
  if (!chunkRes.ok()) return null;

  return { id: doc.id, title, chunks };
}
