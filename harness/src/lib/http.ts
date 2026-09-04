export interface TextResponse {
  url: string;
  status: number;
  ok: boolean;
  headers: Record<string, string>;
  body: string;
}

export interface JsonResponse<T> extends Omit<TextResponse, "body"> {
  body: T;
}

function headersToObject(headers: Headers) {
  const result: Record<string, string> = {};
  headers.forEach((value, key) => {
    result[key] = value;
  });
  return result;
}

export async function getText(url: string, timeoutMs: number): Promise<TextResponse> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(url, {
      method: "GET",
      signal: controller.signal,
      redirect: "follow",
    });

    return {
      url: response.url,
      status: response.status,
      ok: response.ok,
      headers: headersToObject(response.headers),
      body: await response.text(),
    };
  } finally {
    clearTimeout(timer);
  }
}

export async function getJson<T>(url: string, timeoutMs: number): Promise<JsonResponse<T>> {
  const response = await getText(url, timeoutMs);
  let body: T;

  try {
    body = JSON.parse(response.body) as T;
  } catch {
    throw new Error(`${url} did not return valid JSON`);
  }

  return { ...response, body };
}
