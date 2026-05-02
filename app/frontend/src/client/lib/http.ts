export async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url, { headers: { Accept: 'application/json' } });
  const data = (await res.json().catch(() => ({}))) as T;
  if (!res.ok) {
    const err = (data as { error?: string }).error || 'Ошибка запроса';
    throw new Error(err);
  }
  return data;
}

export async function postJson<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body || {})
  });
  const data = (await res.json().catch(() => ({}))) as T;
  if (!res.ok) {
    const err = (data as { error?: string }).error || 'Ошибка запроса';
    throw new Error(err);
  }
  return data;
}
