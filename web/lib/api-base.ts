import { getToken, logout as authLogout } from "./auth";

export const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers || {}),
    },
  });
  // 401 with a token means the token is invalid or expired; clear it and redirect.
  // 401 without a token (e.g. login with wrong password) is a normal auth error.
  if (res.status === 401) {
    if (token) {
      authLogout();
      throw new ApiError("session expired", 401);
    }
    const errJson = await res.json().catch(() => ({}));
    throw new ApiError(errJson.message || "invalid credentials", 401);
  }
  // 204 No Content: success with no body (e.g. DELETE).
  if (res.status === 204) {
    return undefined as T;
  }
  const json = await res.json().catch(() => ({}));
  if (!res.ok || json.code !== 0) {
    throw new ApiError(json.message || `request failed: ${res.status}`, res.status);
  }
  return json.data as T;
}

// fetchOriginal downloads the uploaded file bytes (PDF.js document loading).
// An optional byte Range enables incremental loading of large files.
export async function fetchOriginal(
  kbId: string,
  docId: string,
  range?: { start: number; end: number }
): Promise<ArrayBuffer> {
  const token = getToken();
  const res = await fetch(
    `${API_BASE}/api/v1/knowledge-bases/${kbId}/documents/${docId}/original`,
    {
      credentials: "include",
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(range ? { Range: `bytes=${range.start}-${range.end}` } : {}),
      },
    }
  );
  if (!res.ok) throw new ApiError(`load original failed: ${res.status}`, res.status);
  return await res.arrayBuffer();
}

// streamSSE opens a POST request and parses the Server-Sent Events response.
// Used by chat streaming. The fetch ReadableStream is read incrementally so
// tokens appear in the UI as soon as the server emits them.
export async function* streamSSE(path: string, body: unknown, signal?: AbortSignal): AsyncGenerator<unknown> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    const txt = await res.text().catch(() => "");
    throw new ApiError(txt || `stream failed: ${res.status}`, res.status);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    // SSE events are separated by a blank line. Split on "\n\n" boundaries.
    const events = buffer.split("\n\n");
    buffer = events.pop() || "";
    for (const evt of events) {
      const line = evt.split("\n").find((l) => l.startsWith("data:"));
      if (!line) continue;
      const payload = line.slice(5).trim();
      if (!payload) continue;
      try {
        yield JSON.parse(payload);
      } catch {
        // Skip malformed events; the server keeps the stream open.
      }
    }
  }
}

// streamGETSSE opens a GET Server-Sent Events connection and parses it
// incrementally. Uses fetch streaming (rather than the native EventSource)
// because the JWT must be sent via the Authorization header, which
// EventSource cannot set. Same SSE framing as streamSSE.
export async function* streamGETSSE(path: string, signal?: AbortSignal): AsyncGenerator<unknown> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    method: "GET",
    credentials: "include",
    headers: {
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal,
  });
  if (!res.ok || !res.body) {
    const txt = await res.text().catch(() => "");
    throw new ApiError(txt || `stream failed: ${res.status}`, res.status);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split("\n\n");
    buffer = events.pop() || "";
    for (const evt of events) {
      const line = evt.split("\n").find((l) => l.startsWith("data:"));
      if (!line) continue;
      const payload = line.slice(5).trim();
      if (!payload) continue;
      try {
        yield JSON.parse(payload);
      } catch {
        // Skip malformed events.
      }
    }
  }
}
