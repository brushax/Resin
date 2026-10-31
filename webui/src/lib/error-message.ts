import { ApiError, NetworkError, TimeoutError } from "./api-client";

type TranslateFn = (text: string, options?: Record<string, unknown>) => string;

// Engine-level fetch messages. WebKit says "Load failed", Chromium says
// "Failed to fetch", and iOS also reports offline states with its own wording.
const NETWORK_ERROR_PATTERN =
  /load failed|failed to fetch|networkerror|network connection|network request failed|the internet connection appears to be offline|a server with the specified hostname could not be found|the request timed out|timed out/i;

export const NETWORK_ERROR_MESSAGE = "无法连接 Resin 服务器，请检查网络后重试";
export const TIMEOUT_ERROR_MESSAGE = "请求超时，服务器长时间未响应，请稍后重试";

export function isNetworkErrorMessage(message: string | undefined | null): boolean {
  return Boolean(message && NETWORK_ERROR_PATTERN.test(message));
}

/**
 * True for failures worth retrying: dropped connections and timeouts, plus 5xx
 * responses. Client errors (4xx) are deterministic and retrying only amplifies
 * load on a phone connection that is already struggling.
 */
export function isTransientError(error: unknown): boolean {
  if (error instanceof TimeoutError || error instanceof NetworkError) {
    return true;
  }
  if (error instanceof ApiError) {
    return error.status >= 500 || error.status === 408 || error.status === 429;
  }
  if (error instanceof Error) {
    return isNetworkErrorMessage(error.message);
  }
  return false;
}

export function formatApiErrorMessage(error: unknown, t: TranslateFn): string {
  if (error instanceof TimeoutError) {
    return t(TIMEOUT_ERROR_MESSAGE);
  }
  if (error instanceof NetworkError) {
    return t(NETWORK_ERROR_MESSAGE);
  }
  if (error instanceof ApiError) {
    const message = error.message ? t(error.message) : t("未知错误");
    return error.code ? `${error.code}: ${message}` : message;
  }
  if (error instanceof Error) {
    if (isNetworkErrorMessage(error.message)) {
      return t(NETWORK_ERROR_MESSAGE);
    }
    return error.message ? t(error.message) : t("未知错误");
  }
  return t("未知错误");
}
