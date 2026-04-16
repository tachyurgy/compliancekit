/**
 * Thin fetch wrapper that talks to the ComplianceKit backend.
 *
 * - Prepends `VITE_API_BASE_URL` to relative paths.
 * - Sends cookies for session auth (`credentials: 'include'`).
 * - Parses JSON responses.
 * - Normalizes error envelopes `{ error: { code, message } }` into `ApiError`.
 * - Redirects to `/login` on 401 (except for explicitly public endpoints).
 */

export interface ApiErrorPayload {
  code: string;
  message: string;
  details?: unknown;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: unknown;

  constructor(status: number, payload: ApiErrorPayload) {
    super(payload.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = payload.code;
    this.details = payload.details;
  }
}

const API_BASE_URL: string =
  import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export interface ApiFetchOptions extends RequestInit {
  /** If true, don't redirect to /login on a 401. Useful for the session probe. */
  skipAuthRedirect?: boolean;
  /** If provided, object is JSON-stringified and Content-Type is set. */
  json?: unknown;
  /** Query string params, serialized safely. */
  query?: Record<string, string | number | boolean | null | undefined>;
}

function buildUrl(path: string, query?: ApiFetchOptions['query']): string {
  const base = path.startsWith('http') ? path : `${API_BASE_URL}${path}`;
  if (!query) return base;
  const usp = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === null || v === undefined) continue;
    usp.append(k, String(v));
  }
  const qs = usp.toString();
  return qs ? `${base}${base.includes('?') ? '&' : '?'}${qs}` : base;
}

export async function apiFetch<T = unknown>(
  path: string,
  options: ApiFetchOptions = {},
): Promise<T> {
  const { json, query, skipAuthRedirect, headers, body, ...rest } = options;

  const url = buildUrl(path, query);
  const finalHeaders = new Headers(headers);
  if (json !== undefined) {
    finalHeaders.set('Content-Type', 'application/json');
  }
  if (!finalHeaders.has('Accept')) {
    finalHeaders.set('Accept', 'application/json');
  }

  const response = await fetch(url, {
    ...rest,
    headers: finalHeaders,
    credentials: 'include',
    body: json !== undefined ? JSON.stringify(json) : body,
  });

  // 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  const contentType = response.headers.get('content-type') ?? '';
  const isJson = contentType.includes('application/json');
  const payload = isJson ? await response.json().catch(() => null) : null;

  if (!response.ok) {
    if (response.status === 401 && !skipAuthRedirect) {
      // Preserve where the user was trying to go.
      const current = window.location.pathname + window.location.search;
      if (!current.startsWith('/login')) {
        window.location.assign(
          `/login?next=${encodeURIComponent(current)}`,
        );
      }
    }
    const errEnvelope =
      (payload && typeof payload === 'object' && 'error' in payload
        ? (payload as { error: ApiErrorPayload }).error
        : null) ?? {
        code: `http_${response.status}`,
        message: response.statusText || 'Request failed',
      };
    throw new ApiError(response.status, errEnvelope);
  }

  return (payload ?? (undefined as unknown)) as T;
}

export { API_BASE_URL };
