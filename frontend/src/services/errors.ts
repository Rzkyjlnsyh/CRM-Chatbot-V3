export function apiErrorMessage(error: unknown, fallback: string): string {
  if (typeof error !== 'object' || error === null) return fallback;
  const response = (error as { response?: unknown }).response;
  if (typeof response !== 'object' || response === null) return fallback;
  const data = (response as { data?: unknown }).data;
  if (typeof data !== 'object' || data === null) return fallback;
  const message = (data as { error?: unknown }).error;
  return typeof message === 'string' && message.trim() ? message : fallback;
}
