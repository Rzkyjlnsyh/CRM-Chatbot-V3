import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { trackMetaEvent } from '../services/metaPixel';

export default function MetaPixelTracker() {
  const location = useLocation();

  useEffect(() => {
    if (location.pathname.startsWith('/admin') || location.pathname.startsWith('/app')) return;
    void trackMetaEvent('PageView');
  }, [location.pathname, location.search]);

  return null;
}
