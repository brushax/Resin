import { QueryClient } from "@tanstack/react-query";
import { isTransientError } from "./error-message";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      // Retry dropped connections and 5xx twice with backoff. A single retry
      // was not enough to survive a phone switching between Wi-Fi and cellular,
      // while 4xx responses were being retried pointlessly.
      retry: (failureCount, error) => failureCount < 2 && isTransientError(error),
      retryDelay: (attemptIndex) => Math.min(1_000 * 2 ** attemptIndex, 8_000),
      refetchOnWindowFocus: true,
    },
    mutations: {
      // Do not replay writes: the server may already have applied them.
      retry: 0,
    },
  },
});
