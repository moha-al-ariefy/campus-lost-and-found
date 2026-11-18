export class ApiError extends Error {
  constructor(public code: string, public status = 0, public fields: string[] = [], public claimId = '') { super(code); }
}

export async function photoSubmissionRequest(key: string, data: unknown, photo: File | null): Promise<RequestInit> {
  const metadata = JSON.stringify(data);
  if (!photo) return { method: 'POST', headers: { 'Idempotency-Key': key }, body: metadata };
  const multipart = new FormData();
  multipart.append('item', metadata);
  multipart.append('photo', photo, 'photo');
  // WebView2 can omit disk-backed files when its desktop handler reads multipart requests.
  const encoded = new Response(multipart);
  const headers = new Headers(encoded.headers);
  headers.set('Idempotency-Key', key);
  try {
    return { method: 'POST', headers, body: await encoded.arrayBuffer() };
  } catch {
    throw new ApiError('photoReadFailed', 400);
  }
}

export function photoFileError(file: File): 'photoTooLarge' | 'invalidPhoto' | undefined {
  if (file.size > 10_000_000) return 'photoTooLarge';
  if (file.size === 0 || (file.type && !['image/jpeg', 'image/png'].includes(file.type))) return 'invalidPhoto';
  return undefined;
}
