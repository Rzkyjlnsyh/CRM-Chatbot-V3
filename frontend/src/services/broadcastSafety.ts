import type { BroadcastSafetyForm } from '../types';

export function defaultBroadcastSafetyForm(): BroadcastSafetyForm {
  return {
    consent_category: 'marketing',
    consent_confirmed: false,
    consent_source: '',
    consent_granted_at: '',
    consent_note: '',
    risk_acknowledged: false,
    override_phrase: '',
    override_reason: '',
  };
}
