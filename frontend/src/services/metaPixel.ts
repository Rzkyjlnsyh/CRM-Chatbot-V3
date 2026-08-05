export type MetaBrowserContext = {
  event_id: string;
  event_source_url: string;
  fbp: string;
  fbc: string;
};

type MetaPixelConfig = { enabled: boolean; pixel_id: string };

interface MetaFbq {
  (...args: unknown[]): void;
  callMethod?: (...args: unknown[]) => void;
  queue: unknown[][];
  loaded: boolean;
  version: string;
  push: MetaFbq;
}

declare global {
  interface Window {
    fbq?: MetaFbq;
    _fbq?: MetaFbq;
  }
}

let configPromise: Promise<MetaPixelConfig | null> | null = null;
let initializedPixelID = '';

function getCookie(name: string): string {
  const prefix = `${name}=`;
  return document.cookie
    .split(';')
    .map(value => value.trim())
    .find(value => value.startsWith(prefix))
    ?.slice(prefix.length) || '';
}

function decodeCookie(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return '';
  }
}

function persistFacebookClickID(): string {
  const current = getCookie('_fbc');
  if (current) return current;
  const clickID = new URLSearchParams(window.location.search).get('fbclid');
  if (!clickID) return '';
  const value = `fb.1.${Date.now()}.${clickID}`;
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `_fbc=${encodeURIComponent(value)}; Path=/; Max-Age=7776000; SameSite=Lax${secure}`;
  return value;
}

function fetchMetaPixelConfig(): Promise<MetaPixelConfig | null> {
  if (!configPromise) {
    configPromise = fetch('/api/meta/pixel-config', { headers: { Accept: 'application/json' } })
      .then(async response => response.ok ? await response.json() as MetaPixelConfig : null)
      .catch(() => null);
  }
  return configPromise;
}

function installMetaPixel(): MetaFbq {
  if (window.fbq) return window.fbq;
  const fbq = ((...args: unknown[]) => {
    if (fbq.callMethod) fbq.callMethod(...args);
    else fbq.queue.push(args);
  }) as MetaFbq;
  fbq.push = fbq;
  fbq.loaded = true;
  fbq.version = '2.0';
  fbq.queue = [];
  window.fbq = fbq;
  window._fbq = fbq;

  if (!document.querySelector('script[data-meta-pixel]')) {
    const script = document.createElement('script');
    script.async = true;
    script.dataset.metaPixel = 'true';
    script.src = 'https://connect.facebook.net/en_US/fbevents.js';
    document.head.appendChild(script);
  }
  return fbq;
}

async function ensureMetaPixel(): Promise<{ fbq: MetaFbq; pixelID: string } | null> {
  const config = await fetchMetaPixelConfig();
  if (!config?.enabled || !config.pixel_id) return null;
  const fbq = installMetaPixel();
  if (initializedPixelID !== config.pixel_id) {
    fbq('init', config.pixel_id);
    initializedPixelID = config.pixel_id;
  }
  return { fbq, pixelID: config.pixel_id };
}

export function createMetaEventID(prefix: string): string {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}_${Math.random().toString(36).slice(2)}`;
  return `${prefix}_${random}`.slice(0, 100);
}

export function getMetaBrowserContext(eventID: string): MetaBrowserContext {
  return {
    event_id: eventID,
    event_source_url: window.location.href,
    fbp: decodeCookie(getCookie('_fbp')),
    fbc: decodeCookie(persistFacebookClickID()),
  };
}

export async function trackMetaEvent(
  eventName: string,
  parameters: Record<string, unknown> = {},
  eventID = createMetaEventID(eventName.toLowerCase()),
): Promise<void> {
  const pixel = await ensureMetaPixel();
  if (!pixel) return;
  pixel.fbq('trackSingle', pixel.pixelID, eventName, parameters, { eventID });
}
