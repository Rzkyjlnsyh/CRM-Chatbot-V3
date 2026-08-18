// Tipe data (internal company — tanpa langganan/paket).

export interface Tenant {
  id: number;
  name: string;
  created_at: string;
}

export interface Usage {
  tenant: Tenant;
  period: string;
  numbers_used: number;
  max_numbers: number;
  ai_replies_used: number;
  ai_replies_max: number;
  broadcast_used: number;
  broadcast_max: number;
}

export interface TenantRow extends Tenant {
  numbers_used: number;
  owner_name?: string;
  owner_email?: string;
  owner_phone?: string;
}

export interface AdminStats {
  total_tenants: number;
  total_agents: number;
  total_chats: number;
}

export interface AIPreset {
  key: string;
  label: string;
  model: string;
  available: boolean; // API key sudah diisi di .env
}

export interface AIModelConfig {
  active: string;
  presets: AIPreset[];
}

export interface MetaTrackingEventStatus {
  id: number;
  event_id: string;
  event_name: string;
  status: 'pending' | 'sending' | 'sent' | 'failed';
  attempts: number;
  last_error?: string;
  sent_at?: string;
  created_at: string;
}

export interface MetaTrackingAdminConfig {
  enabled: boolean;
  pixel_id: string;
  graph_version: string;
  test_event_code: string;
  token_configured: boolean;
  stats: {
    pending: number;
    sent: number;
    failed: number;
    last_event?: MetaTrackingEventStatus;
  };
}

export interface ChatMsg {
  id: number;
  sender: string;
  message: string;
  reply: string;
  from_human: boolean;
  media_type: string; // "", image, document, audio, video, sticker
  file_name: string;
  mimetype: string;
  image_analysis?: string;
  image_analysis_status?: 'completed' | 'failed' | string;
  image_analysis_model?: string;
  image_analysis_confidence?: number;
  image_analysis_answer?: string;
  image_analysis_product_id?: number;
  image_analysis_needs_human?: boolean;
  wa_msg_id?: string;
  reply_to?: string;
  reply_text?: string;
  revoked?: boolean;
  created_at: string;
}

export interface Contact {
  sender: string;
  last_at: string;
  last_msg?: string;
  needs_human: boolean;
  manual_pause_until?: string | null;
  name?: string;
}

/** Ringkasan operasional percakapan untuk CS di inbox (bukan dikirim ke pelanggan). */
export interface ConversationBrief {
  contact_hint: string;
  intent: string;
  products: string[];
  key_facts: string[];
  open_items: string[];
  risk_flags: string[];
  stage: string;
  summary: string;
  source: string;
  message_count: number;
  last_chat_id: number;
  updated_at: string;
  needs_human: boolean;
  stale: boolean;
  confidence: number;
}

export interface Analytics {
  total_incoming: number;
  ai_replies: number;
  human_replies: number;
  contacts: number;
  open_handoffs: number;
  ai_handled_pct: number;
  trend: { day: string; count: number }[];
}

export interface AIMetrics {
  total_incoming: number;
  ai_replies: number;
  escalated: number;
  escalation_rate: number;
  tool_shipping_success: number;
  tool_shipping_error: number;
  closing_detected: number;
  closing_exported: number;
  ai_errors: number;
  knowledge_hits?: number;
  knowledge_hit_rate?: number;
  avg_answer_overlap?: number;
  avg_top_similarity?: number;
  grounding_retried?: number;
  grounding_fallback?: number;
  product_hits?: number;
  trend: { date: string; total: number; escalated: number }[];
}

export interface NumberCheck {
  input: string;
  number: string;
  registered: boolean;
  warm?: boolean; // pernah chat dengan agent ini
}

export interface CheckResult {
  data: NumberCheck[];
  summary: { sent_today: number; daily_cap: number };
}

export type BroadcastConsentCategory = 'marketing' | 'order_update' | 'reminder' | 'service_info';

export interface BroadcastSafetyForm {
  consent_category: BroadcastConsentCategory;
  consent_confirmed: boolean;
  consent_source: '' | 'form' | 'checkout' | 'customer_request' | 'event' | 'other';
  consent_granted_at: string;
  consent_note: string;
  risk_acknowledged: boolean;
  override_phrase: string;
  override_reason: string;
}

export interface BroadcastGuardFinding {
  code: string;
  severity: 'info' | 'warning' | 'danger' | 'blocked';
  message: string;
  recommendation?: string;
}

export interface BroadcastAssessment {
  level: 'low' | 'medium' | 'high' | 'blocked';
  title: string;
  can_proceed: boolean;
  can_override: boolean;
  requires_acknowledgement: boolean;
  requires_override: boolean;
  total_recipients: number;
  eligible_recipients: number;
  sendable_today: number;
  existing_consent: number;
  consent_to_record: number;
  missing_consent: number;
  opted_out: number;
  engaged_recipients: number;
  no_interaction: number;
  sent_today: number;
  daily_limit: number;
  findings: BroadcastGuardFinding[];
  override_phrase?: string;
}

export interface BroadcastConsentSummary {
  active_consent: number;
  marketing_consent: number;
  interacted: number;
  opted_out: number;
}

export interface BroadcastPreflightBody extends BroadcastSafetyForm {
  message: string;
  recipients: { number: string; name: string }[];
  run_at?: string;
}

export interface Broadcast {
  id: number;
  message: string;
  status: string; // pending, running, resuming, wa_restricted, done, failed, interrupted
  pause_reason?: string;
  pause_code?: number;
  paused_at?: string;
  agent_ids?: string;
  quarantine_json?: string;
  total: number;
  sent: number;
  failed: number;
  skipped: number;
  media_type: string;
  file_name: string;
  consent_category?: BroadcastConsentCategory;
  consent_source?: string;
  risk_level?: BroadcastAssessment['level'];
  risk_acknowledged?: boolean;
  override_reason?: string;
  created_at: string;
}

export interface BroadcastRecipient {
  id: number;
  number: string;
  name: string;
  agent_id?: number;
  status: string; // pending, sent, failed, skipped
  error: string;
  sent_at: string | null;
}

/** Status karantina satu nomor CS dalam rotasi Blast. */
export interface BroadcastQuarantineEntry {
  agent_id: number;
  name: string;
  number: string;
  role: 'primary' | 'backup' | string;
  reason: 'hard' | 'soft' | 'disconnect' | string;
  reason_label: string;
  code?: number;
  at?: string;
  until?: string | null;
  active: boolean;
  advice: string;
  pending_count?: number;
  sent_count?: number;
}

export interface BroadcastRotationAgent {
  id: number;
  name: string;
  number: string;
  role: 'primary' | 'backup' | string;
  connected: boolean;
  pending_count: number;
  sent_count: number;
  failed_count: number;
  skipped_count: number;
  quarantine?: Omit<BroadcastQuarantineEntry, 'agent_id' | 'name' | 'number' | 'role' | 'pending_count' | 'sent_count'> & {
    reason: string;
    reason_label: string;
    active: boolean;
    advice: string;
  };
}

export interface BroadcastRotationSummary {
  enabled: boolean;
  pool_size: number;
  agents: BroadcastRotationAgent[];
  quarantine: BroadcastQuarantineEntry[];
  quarantine_active: number;
  pause_code?: number;
  pause_reason?: string;
}

export interface BroadcastDetailData {
  broadcast: Broadcast;
  recipients: BroadcastRecipient[];
  rotation?: BroadcastRotationSummary;
}

export interface AutoReply {
  id: number;
  keywords: string;
  match_type: string; // contains, exact, prefix
  reply: string;
  enabled: boolean;
  sort_order: number;
}

export interface Template {
  id: number;
  title: string;
  body: string;
  sort_order: number;
}

export type LeadStage = 'new' | 'cold' | 'warm' | 'hot' | 'customer' | 'unqualified';

export interface SavedContact {
  id: number;
  number: string;
  name: string;
  notes: string;
  tags: string; // dipisah koma
  lead_stage: LeadStage;
  lead_stage_source: 'system' | 'ai' | 'activity' | 'manual' | string;
  lead_stage_reason: string;
  lead_stage_confidence: number;
  lead_stage_locked: boolean;
  lead_stage_updated_at: string | null;
  last_at: string | null;
}

export interface SavedContactsResp {
  data: SavedContact[];
  total: number;
  page: number;
  limit: number;
  all_tags: string[];
  stage_counts: Record<LeadStage, number>;
}

export interface FollowUpStep {
  id?: number;
  step_order?: number;
  delay_hours: number;
  message: string;
  ai_generated?: boolean;
  ai_instruction?: string;
}

export interface FollowUp {
  id: number;
  name: string;
  enabled: boolean;
  stop_on_reply: boolean;
  steps: FollowUpStep[];
  counts: { active: number; completed: number; stopped: number };
}

export interface Product {
  id: number;
  agent_id: number;
  name: string;
  product_type?: ProductType;
  price: string;
  description: string;
  details_json?: string;
  knowledge?: string;
  ai_sales_guidance?: string;
  image_path: string;
  image_mime: string;
  image_url?: string;
  buttons_json?: string;
  checkout_steps_json?: string;
  checkout_handoff?: boolean;
  checkout_success_message?: string;
}

export type ProductType = 'physical' | 'digital' | 'service' | 'subscription' | 'event' | 'donation' | 'other';

export interface ProductDetailItem {
  label: string;
  value: string;
}

export type ProductButtonAction = 'checkout' | 'ai' | 'reply' | 'handoff';

export interface ProductButtonConfig {
  key: string;
  label: string;
  icon?: string;
  action: ProductButtonAction;
  response?: string;
}

export type CheckoutStepType = 'text' | 'number' | 'select';

export interface CheckoutStepConfig {
  key: string;
  label: string;
  type: CheckoutStepType;
  required: boolean;
  options?: string[];
}

export interface ProductOrder {
  id: number;
  product_id: number;
  sender: string;
  order_code: string;
  status: string;
  data_json: string;
  summary: string;
  created_at: string;
  product?: Product;
}

export type AIFormStepType = 'text' | 'number' | 'select';

export interface AIFormStepConfig {
  key: string;
  label: string;
  type: AIFormStepType;
  required: boolean;
  options?: string[];
}

export interface AIForm {
  id: number;
  agent_id: number;
  name: string;
  goal: string;
  intent_hints_json: string;
  steps_json: string;
  enabled: boolean;
  handoff: boolean;
  success_message: string;
  created_at: string;
}

export interface AIFormSubmission {
  id: number;
  form_id: number;
  sender: string;
  code: string;
  status: string;
  data_json: string;
  summary: string;
  created_at: string;
  form?: AIForm;
}

export interface WAGroup {
  jid: string;
  name: string;
  participants: number;
  bot_is_admin: boolean;
  guard_enabled?: boolean;
}

export interface GroupGuardConfig {
  id?: number;
  group_jid: string;
  group_name: string;
  enabled: boolean;
  delete_spam: boolean;
  flag_for_kick: boolean;
  auto_kick: boolean;
  block_links: boolean;
  block_phones: boolean;
  block_words: string;
  flood_count: number;
  flood_window_sec: number;
  allow_numbers: string;
}

export interface GroupModerationLog {
  id: number;
  group_jid: string;
  group_name: string;
  sender: string;
  sender_name: string;
  action: string;
  reason: string;
  excerpt: string;
  status: string;
  created_at: string;
}

export interface LabelInfo {
  label_id: string;
  name: string;
  color: number;
  count: number;
}

export interface ScheduledMessage {
  id: number;
  run_at: string;
  message: string;
  product_id?: number;
  product_buttons_json?: string;
  target_type?: 'number' | 'group'; // "group" = pesan diposting ke dalam grup
  recipient_count: number;
  media_type: string;
  file_name: string;
  status: string; // scheduled, running, done, failed, cancelled, interrupted
  consent_category?: BroadcastConsentCategory;
  consent_source?: string;
  risk_level?: BroadcastAssessment['level'];
  risk_acknowledged?: boolean;
  override_reason?: string;
  broadcast_id?: number | null;
}

export function normalizePhone(s: string): string {
  const d = (s.match(/\d/g) || []).join('');
  if (!d) return '';
  if (d.startsWith('0')) return '62' + d.slice(1);
  if (d.startsWith('8')) return '62' + d;
  return d;
}

export interface User {
  id: number;
  name: string;
  username: string;
  email: string;
  role: string;
  is_super_admin: boolean;
  tenant_id: number | null;
}

export function currentUser(): User | null {
  try {
    return JSON.parse(localStorage.getItem('user') || 'null');
  } catch {
    return null;
  }
}

export interface Agent {
  id: number;
  name?: string;
  number?: string;
  system_prompt?: string;
  tone?: string;
  ai_enabled?: boolean;
  auto_read?: boolean;
  ai_reply_delay_min?: number;
  ai_reply_delay_max?: number;
  greeting_enabled?: boolean;
  greeting_message?: string;
  business_hours_enabled?: boolean;
  business_start?: string;
  business_end?: string;
  away_message?: string;
  spreadsheet_url?: string;
  spreadsheet_sheet_name?: string;
  sheet_sync_enabled?: boolean;
  origin_city_id?: number;
  origin_city_name?: string;
  default_weight_gram?: number;
  enabled_couriers?: string;
}

export interface KnowledgeItem {
  id: number;
  question: string;
  answer: string;
  tags?: string;
  source?: string;
  source_url?: string;
}

export interface CrawlJob {
  id: number;
  root_url: string;
  domain: string;
  status: 'pending' | 'crawling' | 'training' | 'stopping' | 'done' | 'failed';
  pages_found: number;
  error?: string;
  persona_updated?: boolean;
  persona_error?: string;
}

export interface CrawlPage {
  id: number;
  url: string;
  title: string;
  status: 'found' | 'crawled' | 'training' | 'trained' | 'skipped' | 'failed';
  char_count: number;
  recommended: boolean;
  /** 0–100 skor multi-sinyal kelayakan knowledge CS */
  recommend_score?: number;
  /** skip | weak | good | strong */
  recommend_tier?: string;
  /** Alasan singkat dipisah " · " */
  recommend_reason?: string;
  error?: string;
}

export interface KnowledgeUsage {
  used_chars: number;
  max_chars: number;
  max_pages: number;
  total_knowledge: number;
  semantic_search?: boolean;
  embedded_knowledge?: number;
}

export interface Handoff {
  id: number;
  sender: string;
  last_msg: string;
}

export function rupiah(n: number): string {
  return 'Rp ' + (n || 0).toLocaleString('id-ID');
}

export interface FlowOption {
  key: string;
  label?: string;
  action: 'reply' | 'reply_menu' | 'goto' | 'handoff';
  reply?: string;
  target?: string;
}
export interface FlowNode {
  message: string;
  options: FlowOption[];
}
export interface Flow {
  agent_id?: number;
  enabled: boolean;
  trigger: string;
  match_type: string;
  display_mode: 'auto' | 'text' | 'buttons';
  structure: string;
  delay_min: number;
  delay_max: number;
}

export interface ApiSettings {
  allowed: boolean;
  connected: boolean;
  has_key: boolean;
  key_hint: string;
  webhook_url: string;
  has_webhook_secret: boolean;
  webhook_secret_hint: string;
}

export interface ScheduledStatus {
  id: number;
  run_at: string;
  text: string;
  media_type: string;
  status: string;
  error?: string;
  created_at: string;
}

// --- Learning Engine types ---

export interface LearningRun {
  id: number;
  agent_id: number;
  status: string; // pending, running, completed, failed
  source_start_date?: string;
  source_end_date?: string;
  total_chats: number;
  human_chats: number;
  pattern_count: number;
  style_profile?: string; // JSON
  summary?: string;       // rekap apa yg dipelajari & diterapkan
  error?: string;
  created_at: string;
  completed_at?: string;
}

export interface LearningPattern {
  id: number;
  learning_run_id: number;
  agent_id: number;
  label_id?: string;   // label WhatsApp konteks pola ("" = umum)
  label_name?: string; // nama label (untuk tampilan)
  pattern_type: string; // greeting, closing, objection_handling, upsell, tone, emoji_style, phrase, follow_up, label_handling, closing_path
  trigger_context: string;
  response_template: string;
  emoji_signature: string;
  confidence: number;
  usage_count: number;
  closing_impact: number;
  status: string; // suggested, applied, rejected
  applied_at?: string;
  knowledge_id?: number;
  created_at: string;
}

export interface LearningSnapshot {
  id: number;
  agent_id: number;
  learning_run_id?: number;
  snapshot_type: string;
  label: string;
  persona_at: string;
  knowledge_count: number;
  created_at: string;
}

export interface LearningConfig {
  id: number;
  agent_id: number;
  enabled: boolean;
  auto_apply: boolean;
  min_confidence: number;
  min_usage_count: number;
  max_patterns_per_run: number;
  preserve_manual_knowledge: boolean;
  schedule_enabled: boolean;
  schedule_cron: string;
  lookback_days: number;
  updated_at: string;
}

export interface LearningStatus {
  last_run?: LearningRun;
  patterns_suggested: number;
  patterns_applied: number;
  patterns_rejected: number;
  snapshot_count: number;
  config: LearningConfig;
}

export interface LearningRunDetail {
  run: LearningRun;
  patterns: LearningPattern[];
}

export interface StyleProfile {
  greeting_patterns: string[];
  closing_patterns: string[];
  common_phrases: string[];
  emoji_usage: string[];
  tone_description: string;
  pacing_style: string;
  objection_handling: string[];
  upsell_techniques: string[];
  follow_up_style: string[];
}

export interface LearningEnqueued {
  run_id: number;
  status: string; // pending
  message: string;
}
