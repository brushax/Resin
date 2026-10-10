import { getStoredAuthToken } from "../features/auth/auth-store";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL?.trim() ?? "";

type Primitive = string | number | boolean | null;
type JsonValue = Primitive | JsonValue[] | { [key: string]: JsonValue };

export type ApiErrorBody = {
  error?: {
    code?: string;
    message?: string;
  };
};

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly body: ApiErrorBody | null;

  constructor(status: number, code: string, message: string, body: ApiErrorBody | null) {
    super(message);
    this.status = status;
    this.code = code;
    this.body = body;
  }
}

type UnauthorizedHandler = () => void;

let unauthorizedHandler: UnauthorizedHandler | null = null;

// The auth layer registers a handler that drops the stored session and
// redirects to the login page, so a rotated RESIN_ADMIN_TOKEN no longer
// leaves pages stuck in a fake "logged in" state (issue #83).
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler;
}

type RequestOptions = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: JsonValue;
  auth?: boolean;
  token?: string;
  signal?: AbortSignal;
};

function buildURL(path: string): string {
  if (path.startsWith("http://") || path.startsWith("https://")) {
    return path;
  }
  return `${API_BASE_URL}${path}`;
}

async function parseErrorBody(response: Response): Promise<ApiErrorBody | null> {
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return null;
  }

  try {
    return (await response.json()) as ApiErrorBody;
  } catch {
    return null;
  }
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, auth = true, token, signal } = options;
  const headers = new Headers();

  if (body !== undefined) {
    headers.set("Content-Type", "application/json; charset=utf-8");
  }

  if (auth) {
    const resolvedToken = token?.trim() || getStoredAuthToken();
    if (resolvedToken) {
      headers.set("Authorization", `Bearer ${resolvedToken}`);
    }
  }

  const response = await fetch(buildURL(path), {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });

  if (!response.ok) {
    const parsed = await parseErrorBody(response);
    const code = parsed?.error?.code ?? "HTTP_ERROR";
    const message = parsed?.error?.message ?? response.statusText;
    // Skip the handler for login probes, which pass an explicit candidate
    // token and must surface errors inline instead of triggering logout.
    if (response.status === 401 && auth && token === undefined && unauthorizedHandler) {
      unauthorizedHandler();
    }
    throw new ApiError(response.status, code, message, parsed);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return undefined as T;
  }

  return (await response.json()) as T;
}
