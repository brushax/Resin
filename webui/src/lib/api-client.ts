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
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.body = body;
  }
}

// Network failures surface differently per engine ("Load failed" on WebKit,
// "Failed to fetch" on Chromium), so they are wrapped into one stable type
// that the UI can translate instead of printing the engine string verbatim.
export class NetworkError extends Error {
  readonly code = "NETWORK_ERROR";
  readonly cause: unknown;

  constructor(message: string, cause?: unknown) {
    super(message);
    this.name = "NetworkError";
    this.cause = cause;
  }
}

export class TimeoutError extends Error {
  readonly code = "TIMEOUT";
  readonly timeoutMs: number;

  constructor(timeoutMs: number) {
    super(`Request timed out after ${timeoutMs}ms`);
    this.name = "TimeoutError";
    this.timeoutMs = timeoutMs;
  }
}

// Mobile Safari often drops requests when the tab is suspended or the network
// switches between Wi-Fi and cellular. Without a deadline such a request can
// hang until the browser gives up, so every call gets one by default.
const DEFAULT_TIMEOUT_MS = 20_000;

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
  /** Milliseconds before the request is aborted. 0 disables the deadline. */
  timeoutMs?: number;
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
  const { method = "GET", body, auth = true, token, signal, timeoutMs = DEFAULT_TIMEOUT_MS } = options;
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

  const controller = new AbortController();
  let timedOut = false;
  const timer =
    timeoutMs > 0
      ? window.setTimeout(() => {
          timedOut = true;
          controller.abort();
        }, timeoutMs)
      : null;
  const forwardAbort = () => controller.abort();
  if (signal?.aborted) {
    controller.abort();
  } else {
    signal?.addEventListener("abort", forwardAbort);
  }

  let response: Response;
  try {
    response = await fetch(buildURL(path), {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: controller.signal,
    });
  } catch (error) {
    if (timedOut) {
      throw new TimeoutError(timeoutMs);
    }
    if (signal?.aborted) {
      // Caller-initiated cancellation stays an AbortError so React Query can
      // treat it as a no-op instead of an error.
      throw error;
    }
    throw new NetworkError(error instanceof Error ? error.message : String(error), error);
  } finally {
    if (timer !== null) {
      window.clearTimeout(timer);
    }
    signal?.removeEventListener("abort", forwardAbort);
  }

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
