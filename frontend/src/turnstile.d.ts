interface TurnstileAPI {
  render: (selector: string, options: { sitekey: string; callback: (token: string) => void }) => string;
  reset: () => void;
}

interface Window {
  turnstile?: TurnstileAPI;
  __TURNSTILE_SITE_KEY__?: string;
}
