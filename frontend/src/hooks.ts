import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from './services/api';
import type { Analytics, AIMetrics, Contact, ChatMsg, ConversationBrief, Broadcast, BroadcastDetailData, BroadcastSafetyForm, BroadcastConsentSummary, WAGroup, GroupGuardConfig, GroupModerationLog, LabelInfo, ScheduledMessage, AutoReply, Template, SavedContact, SavedContactsResp, LeadStage, FollowUp, Agent, KnowledgeItem, Handoff, CrawlJob, CrawlPage, KnowledgeUsage, ScheduledStatus, ApiSettings, Flow, Product, ProductOrder, AIForm, AIFormSubmission, MediaAsset, LearningStatus, LearningScore, LearningRun, LearningRunDetail, LearningPattern, LearningSnapshot, LearningConfig, MetaConfigData } from './types';

type ContactList = { number: string; name: string }[];

// ---- Tenant ----



// ---- Fitur: analitik, inbox, test chat ----

export function useAgentAnalytics(agentId: number) {
  return useQuery<Analytics>({
    queryKey: ['analytics', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/analytics`)).data,
    enabled: !!agentId,
  });
}

export function useAgentAIMetrics(agentId: number) {
  return useQuery<AIMetrics>({
    queryKey: ['ai-metrics', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/ai-metrics`)).data,
    enabled: !!agentId,
    refetchInterval: 10000,
  });
}

export function useContacts(agentId: number) {
  return useQuery<Contact[]>({
    queryKey: ['contacts', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/contacts`)).data.data,
    enabled: !!agentId,
    // Poll lebih longgar: list kontak tidak perlu real-time ketat.
    refetchInterval: 12_000,
    refetchIntervalInBackground: false,
    staleTime: 5_000,
    placeholderData: (prev) => prev,
  });
}

export function useConversation(agentId: number, sender: string) {
  return useQuery<{ data: ChatMsg[]; needs_human: boolean; manual_pause_until?: string | null; media_token: string; has_more?: boolean; total?: number }>({
    queryKey: ['conversation', agentId, sender],
    queryFn: async () => (await api.get(`/agents/${agentId}/conversation`, { params: { sender } })).data,
    enabled: !!agentId && !!sender,
    // 6s cukup untuk inbox CS; tab background tidak mem-poll.
    refetchInterval: 6_000,
    refetchIntervalInBackground: false,
    staleTime: 2_500,
    placeholderData: (prev) => prev,
  });
}

/** Load older messages (cursor pagination) — append ke conversation yang sudah ada. */
export function useLoadOlderMessages(agentId: number, sender: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (beforeId: number) => {
      const res = await api.get(`/agents/${agentId}/conversation`, { params: { sender, before_id: beforeId, limit: 100 } });
      return res.data as { data: ChatMsg[]; has_more: boolean };
    },
    onSuccess: (result, _beforeId) => {
      qc.setQueryData<{ data: ChatMsg[]; has_more?: boolean }>(['conversation', agentId, sender], (prev) => {
        if (!prev) return { data: result.data, has_more: result.has_more };
        // Merge: older messages di depan (karena result.data adalah asc: lama→baru)
        const existingIds = new Set(prev.data.map((m: ChatMsg) => m.id));
        const newMsgs = result.data.filter((m: ChatMsg) => !existingIds.has(m.id));
        return {
          ...prev,
          data: [...newMsgs, ...prev.data],
          has_more: result.has_more,
        };
      });
    },
  });
}

/** Hapus seluruh thread chat satu kontak dari inbox (riwayat + handoff + memory). */
export function useDeleteInboxConversation(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) =>
      (await api.delete(`/agents/${agentId}/conversation`, { params: { sender } })).data as {
        message: string;
        sender: string;
        deleted_chats: number;
      },
    onSuccess: (_data, sender) => {
      qc.setQueryData<Contact[]>(['contacts', agentId], (prev) =>
        (prev || []).filter((c) => c.sender !== sender),
      );
      qc.removeQueries({ queryKey: ['conversation', agentId, sender] });
      qc.removeQueries({ queryKey: ['conversation-brief', agentId, sender] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
      qc.invalidateQueries({ queryKey: ['handoffs', agentId] });
    },
  });
}

/** Ringkasan CS: fakta penting, open items, risiko — cache di server, auto-rebuild bila stale banyak. */
export function useConversationBrief(agentId: number, sender: string) {
  return useQuery<ConversationBrief>({
    queryKey: ['conversation-brief', agentId, sender],
    queryFn: async () => (await api.get(`/agents/${agentId}/conversation/brief`, { params: { sender } })).data.data,
    enabled: !!agentId && !!sender,
    staleTime: 45_000,
    refetchInterval: 90_000,
    refetchIntervalInBackground: false,
    // Jangan blok UI chat: brief boleh datang belakangan.
    placeholderData: (prev) => prev,
  });
}

export function useRefreshConversationBrief(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) =>
      (await api.post(`/agents/${agentId}/conversation/brief`, { sender })).data.data as ConversationBrief,
    onSuccess: (data, sender) => {
      qc.setQueryData(['conversation-brief', agentId, sender], data);
    },
  });
}

export function useSendMessage(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { to: string; message: string; reply_to?: string; reply_text?: string }) =>
      (await api.post(`/agents/${agentId}/send`, body)).data,
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId, vars.to] });
      qc.invalidateQueries({ queryKey: ['conversation-brief', agentId, vars.to] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

export function useSendMedia(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ to, file, caption }: { to: string; file: File; caption: string }) => {
      const fd = new FormData();
      fd.append('to', to);
      fd.append('caption', caption);
      fd.append('file', file);
      return (await api.post(`/agents/${agentId}/send-media`, fd)).data;
    },
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
      qc.invalidateQueries({ queryKey: ['conversation-brief', agentId, vars.to] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}


export function useRevokeMessage(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ msgId, to }: { msgId: string; to: string }) =>
      (await api.delete('/agents/' + agentId + '/messages/' + msgId, { data: { to } })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
    },
  });
}

/** Fire-and-forget presence typing — tanpa useMutation agar tidak re-render UI tiap keystroke. */
export function postAgentTyping(agentId: number, to: string, active: boolean) {
  return api.post(`/agents/${agentId}/typing`, { to, active }).catch(() => undefined);
}

/** @deprecated Prefer postAgentTyping agar tidak memicu re-render composer. */
export function useSendTyping(agentId: number) {
  return useMutation({
    mutationFn: async ({ to, active }: { to: string; active: boolean }) =>
      postAgentTyping(agentId, to, active),
  });
}

export function useResumeBot(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) => (await api.delete(`/agents/${agentId}/handoffs/${sender}`)).data,
    onSuccess: (_d, sender) => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId, sender] });
      qc.invalidateQueries({ queryKey: ['conversation-brief', agentId, sender] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

export function useReanalyzeImage(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ messageId, instruction }: { messageId: number; instruction: string }) =>
      (await api.post(`/agents/${agentId}/messages/${messageId}/analyze`, { instruction })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

// ---- Broadcast ----

const LIVE_BROADCAST_STATUSES = new Set(['pending', 'running', 'resuming', 'cancel_requested']);

export function useBroadcastConsentSummary(agentId: number) {
  return useQuery<BroadcastConsentSummary>({
    queryKey: ['broadcast-consent-summary', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/broadcast/consent-summary`)).data.data,
    enabled: !!agentId,
    staleTime: 30_000,
  });
}

export function useBroadcasts(agentId: number, page: number) {
  return useQuery<{ data: Broadcast[]; total: number; page: number; limit: number }>({
    queryKey: ['broadcasts', agentId, page],
    queryFn: async () => (await api.get(`/agents/${agentId}/broadcasts`, { params: { page } })).data,
    enabled: !!agentId,
    // Respons cepat ketika ada worker aktif, lebih hemat request saat riwayat diam.
    refetchInterval: query => query.state.data?.data.some(b => LIVE_BROADCAST_STATUSES.has(b.status)) ? 2000 : 10000,
    refetchIntervalInBackground: false,
  });
}

export function useChatContacts(agentId: number) {
  return useMutation({
    mutationFn: async () => (await api.get(`/agents/${agentId}/chat-contacts`)).data.data as { number: string; name: string }[],
  });
}

export function useWAContacts(agentId: number) {
  return useMutation({
    mutationFn: async () => (await api.get(`/agents/${agentId}/wa-contacts`)).data.data as ContactList,
  });
}

export function useGroups(agentId: number) {
  return useMutation({ mutationFn: async () => (await api.get(`/agents/${agentId}/groups`)).data.data as WAGroup[] });
}

// useCheckNumbers memvalidasi apakah nomor terdaftar di WhatsApp (pra-blast).
export interface CheckNumbersResult {
  results: Record<string, boolean>;
  registered: string[];
  not_registered: string[];
  total: number;
  registered_count: number;
}
export function useCheckNumbers(agentId: number) {
  return useMutation({
    mutationFn: async (numbers: string[]) =>
      (await api.post(`/agents/${agentId}/check-numbers`, { numbers })).data.data as CheckNumbersResult,
  });
}

// useManagedGroups = query daftar grup (auto-load) untuk halaman Anti-Spam Grup.
export function useManagedGroups(agentId: number, enabled = true) {
  return useQuery({
    queryKey: ['managed-groups', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/groups`)).data.data as WAGroup[],
    enabled,
    retry: false,
  });
}

export function useGroupConfig(agentId: number, gjid: string, enabled = true) {
  return useQuery({
    queryKey: ['group-config', agentId, gjid],
    queryFn: async () => (await api.get(`/agents/${agentId}/group-config`, { params: { gjid } })).data.data as GroupGuardConfig,
    enabled: enabled && !!gjid,
    retry: false,
  });
}

export function useSaveGroupConfig(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: GroupGuardConfig) => (await api.put(`/agents/${agentId}/group-config`, body)).data.data as GroupGuardConfig,
    onSuccess: (_d, b) => {
      qc.invalidateQueries({ queryKey: ['group-config', agentId, b.group_jid] });
      qc.invalidateQueries({ queryKey: ['managed-groups', agentId] });
    },
  });
}

export function useGroupModeration(agentId: number, enabled = true) {
  return useQuery({
    queryKey: ['group-moderation', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/group-moderation`)).data.data as GroupModerationLog[],
    enabled,
    retry: false,
  });
}

export function useConfirmKick(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (logid: number) => (await api.post(`/agents/${agentId}/group-moderation/${logid}/confirm-kick`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['group-moderation', agentId] }),
  });
}

export function useDismissModeration(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (logid: number) => (await api.post(`/agents/${agentId}/group-moderation/${logid}/dismiss`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['group-moderation', agentId] }),
  });
}

export function useGroupMembers(agentId: number) {
  return useMutation({ mutationFn: async (jid: string) => (await api.get(`/agents/${agentId}/group-members`, { params: { jid } })).data.data as ContactList });
}

export function useLabels(agentId: number) {
  return useMutation({ mutationFn: async () => (await api.post(`/agents/${agentId}/labels/sync`)).data.data as LabelInfo[] });
}

export function useLabelContacts(agentId: number) {
  return useMutation({ mutationFn: async (labelId: string) => (await api.get(`/agents/${agentId}/label-contacts`, { params: { label_id: labelId } })).data.data as ContactList });
}

// ---- Auto-reply (kata kunci) ----

export function useAutoReplies(agentId: number) {
  return useQuery<AutoReply[]>({
    queryKey: ['autoreplies', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/auto-replies`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveAutoReply(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (r: Partial<AutoReply>) =>
      r.id
        ? (await api.put(`/agents/${agentId}/auto-replies/${r.id}`, r)).data
        : (await api.post(`/agents/${agentId}/auto-replies`, r)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['autoreplies', agentId] }),
  });
}

export function useDeleteAutoReply(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (rid: number) => (await api.delete(`/agents/${agentId}/auto-replies/${rid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['autoreplies', agentId] }),
  });
}

// ---- Template pesan (quick reply) ----

export function useTemplates(agentId: number) {
  return useQuery<Template[]>({
    queryKey: ['templates', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/templates`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveTemplate(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (t: Partial<Template>) =>
      t.id
        ? (await api.put(`/agents/${agentId}/templates/${t.id}`, t)).data
        : (await api.post(`/agents/${agentId}/templates`, t)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates', agentId] }),
  });
}

export function useDeleteTemplate(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tid: number) => (await api.delete(`/agents/${agentId}/templates/${tid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates', agentId] }),
  });
}

// ---- Kontak (CRM ringan) ----

export function useCrmContacts(agentId: number, q: string, tag: string, stage: LeadStage | '', page: number) {
  return useQuery<SavedContactsResp>({
    queryKey: ['crm-contacts', agentId, q, tag, stage, page],
    queryFn: async () => (await api.get(`/agents/${agentId}/crm/contacts`, { params: { q, tag, stage, page } })).data,
    enabled: !!agentId,
    refetchInterval: 15000,
  });
}

export function useSaveCrmContact(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (ct: Partial<SavedContact>) =>
      ct.id
        ? (await api.put(`/agents/${agentId}/crm/contacts/${ct.id}`, ct)).data
        : (await api.post(`/agents/${agentId}/crm/contacts`, ct)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

export function useDeleteCrmContact(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (cid: number) => (await api.delete(`/agents/${agentId}/crm/contacts/${cid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// useCrmContactsExport mengambil SEMUA kontak hasil filter (tanpa paginasi),
// dipakai untuk menjadikan satu tag jadi target broadcast.
export function useCrmContactsExport(agentId: number) {
  return useMutation({
    mutationFn: async ({ q, tag, stage }: { q: string; tag: string; stage: LeadStage | '' }) =>
      (await api.get(`/agents/${agentId}/crm/contacts`, { params: { q, tag, stage, all: 1 } })).data.data as SavedContact[],
  });
}

export function useBulkStageCrmContacts(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { ids: number[]; lead_stage: LeadStage }) =>
      (await api.post(`/agents/${agentId}/crm/contacts/bulk-stage`, body)).data as { updated: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// useImportCrmContacts memasukkan banyak kontak sekaligus (manual/terkoneksi/CSV).
export function useImportCrmContacts(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { contacts: { number: string; name: string }[]; tag?: string }) =>
      (await api.post(`/agents/${agentId}/crm/contacts/import`, body)).data as { imported: number; skipped: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// useBulkDeleteCrmContacts menghapus kontak terpilih (ids) atau semua sesuai filter.
export function useBulkDeleteCrmContacts(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { ids?: number[]; all?: boolean; q?: string; tag?: string; stage?: LeadStage | '' }) =>
      (await api.post(`/agents/${agentId}/crm/contacts/bulk-delete`, body)).data as { deleted: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// ---- Follow-up (drip) ----

export function useFollowUps(agentId: number) {
  return useQuery<FollowUp[]>({
    queryKey: ['follow-ups', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/follow-ups`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveFollowUp(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fu: Partial<FollowUp>) =>
      fu.id
        ? (await api.put(`/agents/${agentId}/follow-ups/${fu.id}`, fu)).data
        : (await api.post(`/agents/${agentId}/follow-ups`, fu)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['follow-ups', agentId] }),
  });
}

export function useDeleteFollowUp(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fid: number) => (await api.delete(`/agents/${agentId}/follow-ups/${fid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['follow-ups', agentId] }),
  });
}

export function useEnrollFollowUp(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ fid, recipients }: { fid: number; recipients: { number: string; name: string }[] }) =>
      (await api.post(`/agents/${agentId}/follow-ups/${fid}/enroll`, { recipients })).data as { added: number; skipped: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['follow-ups', agentId] }),
  });
}

// ---- Produk ----

export function useProducts(agentId: number) {
  return useQuery<Product[]>({
    queryKey: ['products', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/products`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveProduct(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, fd }: { id?: number; fd: FormData }) => {
      if (id) return (await api.put(`/agents/${agentId}/products/${id}`, fd)).data;
      return (await api.post(`/agents/${agentId}/products`, fd)).data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['products', agentId] });
      qc.invalidateQueries({ queryKey: ['product-orders', agentId] });
    },
  });
}

export function useGenerateProductAI(agentId: number) {
  return useMutation({
    mutationFn: async (body: {
      name: string;
      product_type: string;
      price: string;
      description: string;
      details_json: string;
      existing_knowledge: string;
      checkout_enabled: boolean;
    }) => (await api.post(`/agents/${agentId}/products/generate-ai`, body)).data as {
      knowledge: string;
      ai_sales_guidance: string;
    },
  });
}

export function useDeleteProduct(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${agentId}/products/${id}`)).data,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['products', agentId] }); },
  });
}

export function useSendProduct(agentId: number) {
  return useMutation({
    mutationFn: async ({ pid, to }: { pid: number; to: string }) =>
      (await api.post(`/agents/${agentId}/products/${pid}/send`, { to })).data,
  });
}

export function useProductOrders(agentId: number) {
  return useQuery<ProductOrder[]>({
    queryKey: ['product-orders', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/product-orders`)).data.data,
    enabled: !!agentId,
    refetchInterval: 15000,
  });
}

export function useAIForms(agentId: number) {
  return useQuery<AIForm[]>({
    queryKey: ['ai-forms', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/ai-forms`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveAIForm(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (form: Partial<AIForm>) =>
      form.id
        ? (await api.put(`/agents/${agentId}/ai-forms/${form.id}`, form)).data
        : (await api.post(`/agents/${agentId}/ai-forms`, form)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ai-forms', agentId] });
      qc.invalidateQueries({ queryKey: ['ai-form-submissions', agentId] });
    },
  });
}

export function useDeleteAIForm(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${agentId}/ai-forms/${id}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['ai-forms', agentId] }),
  });
}

export function useAIFormSubmissions(agentId: number) {
  return useQuery<AIFormSubmission[]>({
    queryKey: ['ai-form-submissions', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/ai-form-submissions`)).data.data,
    enabled: !!agentId,
    refetchInterval: 15000,
  });
}

// ---- Jadwal ----

export function useSchedules(agentId: number) {
  return useQuery<ScheduledMessage[]>({
    queryKey: ['schedules', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/schedules`)).data.data,
    enabled: !!agentId,
    refetchInterval: 10000,
  });
}

export function useCreateSchedule(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fd: FormData) => (await api.post(`/agents/${agentId}/schedule`, fd)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schedules', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast-consent-summary', agentId] });
    },
  });
}

export function useCancelSchedule(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sid: number) => (await api.delete(`/agents/${agentId}/schedule/${sid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['schedules', agentId] }),
  });
}

export function useBroadcastDetail(agentId: number, bid: number | null) {
  return useQuery<BroadcastDetailData>({
    queryKey: ['broadcast', agentId, bid],
    queryFn: async () => (await api.get(`/agents/${agentId}/broadcasts/${bid}`)).data.data,
    enabled: !!agentId && !!bid,
    refetchInterval: query => LIVE_BROADCAST_STATUSES.has(query.state.data?.broadcast.status || '') ? 1500 : false,
    refetchIntervalInBackground: false,
  });
}

export function useCreateBroadcast(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { message: string; recipients: { number: string; name: string }[]; min_delay: number; max_delay: number; rest_every: number; rest_duration: number; file: File | null; safety: BroadcastSafetyForm; agent_ids?: number[]; product_id?: number }) => {
      const fd = new FormData();
      fd.append('message', body.message);
      fd.append('recipients', JSON.stringify(body.recipients));
      fd.append('min_delay', String(body.min_delay));
      fd.append('max_delay', String(body.max_delay));
      fd.append('rest_every', String(body.rest_every));
      fd.append('rest_duration', String(body.rest_duration));
      Object.entries(body.safety).forEach(([key, value]) => fd.append(key, String(value)));
      if (body.file) fd.append('file', body.file);
      if (body.agent_ids && body.agent_ids.length) fd.append('agent_ids', JSON.stringify(body.agent_ids));
      if (body.product_id) fd.append('product_id', String(body.product_id));
      return (await api.post(`/agents/${agentId}/broadcast`, fd)).data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['broadcasts', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast-consent-summary', agentId] });
    },
  });
}

export interface BroadcastRotationTestResult {
  pool_size: number;
  sample_size: number;
  failed_agent_id: number;
  reassigned: number;
  messages_sent: number;
  agents: Array<{
    id: number;
    name?: string;
    number?: string;
    initial_count: number;
    after_failover_count: number;
    simulated_failed: boolean;
  }>;
}

export function useTestBroadcastRotation(agentId: number) {
  return useMutation({
    mutationFn: async (agentIds: number[]) =>
      (await api.post(`/agents/${agentId}/broadcast/rotation-test`, { agent_ids: agentIds })).data.data as BroadcastRotationTestResult,
  });
}

export function useCancelBroadcast(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (bid: number) =>
      (await api.post(`/agents/${agentId}/broadcasts/${bid}/cancel`)).data,
    onSuccess: (_data, bid) => {
      qc.invalidateQueries({ queryKey: ['broadcasts', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast', agentId, bid] });
    },
  });
}

export function useResumeBroadcast(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (bid: number) =>
      (await api.post(`/agents/${agentId}/broadcasts/${bid}/resume`)).data,
    onSuccess: (_data, bid) => {
      qc.invalidateQueries({ queryKey: ['broadcasts', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast', agentId, bid] });
      qc.invalidateQueries({ queryKey: ['schedules', agentId] });
    },
  });
}

// ---- Agent list & detail (Dashboard) ----

export function useAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: async () => (await api.get('/agents')).data.data,
    staleTime: 30_000,
  });
}

export function useAgentStatuses() {
  return useQuery<Record<string, string>>({
    queryKey: ['agent-statuses'],
    queryFn: async () => (await api.get('/agents-status')).data.data,
    refetchInterval: 4000,
  });
}

export function useAgentStatus(agentId: number) {
  return useQuery<{ status: string; qr: string; qr_ttl: number; pair_code: string; pair_error: string; number: string; name: string }>({
    queryKey: ['agent', agentId, 'status'],
    queryFn: async () => (await api.get(`/agents/${agentId}/wa/status`)).data,
    enabled: !!agentId,
    refetchInterval: 4000,
  });
}

export function useAgentHistory(agentId: number) {
  return useQuery<unknown[]>({
    queryKey: ['agent', agentId, 'history'],
    queryFn: async () => (await api.get(`/agents/${agentId}/chat-history`)).data.data,
    enabled: !!agentId,
    refetchInterval: 4000,
  });
}

export function useAgentKnowledge(agentId: number) {
  return useQuery<KnowledgeItem[]>({
    queryKey: ['agent', agentId, 'knowledge'],
    queryFn: async () => (await api.get(`/agents/${agentId}/knowledge`)).data.data,
    enabled: !!agentId,
    refetchInterval: 4000,
  });
}

export function useAgentHandoffs(agentId: number) {
  return useQuery<Handoff[]>({
    queryKey: ['agent', agentId, 'handoffs'],
    queryFn: async () => (await api.get(`/agents/${agentId}/handoffs`)).data.data,
    enabled: !!agentId,
    refetchInterval: 4000,
  });
}

// ---- Mutasi agent (Dashboard) ----

export function useSaveAgent(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Record<string, unknown>) =>
      (await api.put(`/agents/${agentId}`, body)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId] });
    },
  });
}

export function useCreateAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; tone: string }) =>
      (await api.post('/agents', body)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });
}

export function useDeleteAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${id}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });
}

export function useAgentConnect(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/wa/connect`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'status'] }),
  });
}

export function useAgentDisconnect(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/wa/logout`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'status'] }),
  });
}

export function useAddKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { question: string; answer: string; tags: string }) =>
      (await api.post(`/agents/${agentId}/knowledge`, body)).data as { data: KnowledgeItem; merged: boolean },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useDeleteKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${agentId}/knowledge/${id}`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useUpdateKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { id: number; question: string; answer: string; tags: string }) =>
      (await api.put(`/agents/${agentId}/knowledge/${body.id}`, body)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useDeleteAllKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.delete(`/agents/${agentId}/knowledge-all`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useGenerateKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { text: string; count: number }) =>
      (await api.post(`/agents/${agentId}/knowledge/generate`, body)).data as { data: KnowledgeItem[]; knowledge: number },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
      qc.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

// ---- Latih dari Website (crawl) ----

export function useCrawlStatus(agentId: number) {
  return useQuery<{ job: CrawlJob | null; pages: CrawlPage[] }>({
    queryKey: ['agent', agentId, 'crawl'],
    queryFn: async () => (await api.get(`/agents/${agentId}/crawl`)).data,
    enabled: !!agentId,
    // Polling cepat selagi crawl/pelatihan berjalan (termasuk saat dihentikan), berhenti saat idle/selesai.
    refetchInterval: (q) => {
      const s = q.state.data?.job?.status;
      return s === 'pending' || s === 'crawling' || s === 'training' || s === 'stopping' ? 2500 : false;
    },
  });
}

export function useKnowledgeUsage(agentId: number) {
  return useQuery<KnowledgeUsage>({
    queryKey: ['agent', agentId, 'knowledge-usage'],
    queryFn: async () => (await api.get(`/agents/${agentId}/knowledge-usage`)).data,
    enabled: !!agentId,
  });
}

export function useStartCrawl(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (url: string) => (await api.post(`/agents/${agentId}/crawl`, { url })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'crawl'] }),
  });
}

export function useTrainCrawlPages(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { jobId: number; pageIds: number[]; updatePersona: boolean }) =>
      (await api.post(`/agents/${agentId}/crawl/${vars.jobId}/train`, { page_ids: vars.pageIds, update_persona: vars.updatePersona })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'crawl'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useStopTraining(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (jobId: number) =>
      (await api.post(`/agents/${agentId}/crawl/${jobId}/train/stop`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'crawl'] }),
  });
}

export function useRegeneratePersona(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      (await api.post(`/agents/${agentId}/persona/regenerate`)).data as { system_prompt: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });
}

export function useResumeHandoff(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) => (await api.delete(`/agents/${agentId}/handoffs/${sender}`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'handoffs'] });
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

export function useTestChat(agentId: number) {
  return useMutation({
    mutationFn: async (vars: { message: string; history: { role: 'user' | 'bot'; text: string }[] }) =>
      (await api.post(`/agents/${agentId}/test-chat`, vars)).data as {
        reply: string; escalate: boolean; model?: string;
      },
  });
}

// ---- Flow (alur/menu) ----

export function useApiSettings(agentId: number) {
  return useQuery<ApiSettings>({
    queryKey: ['api-settings', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/api`)).data.data,
    enabled: !!agentId,
  });
}

export function useRotateApiKey(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/api/key`)).data as { api_key: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useRevokeApiKey(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.delete(`/agents/${agentId}/api/key`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useSaveWebhook(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (webhook_url: string) =>
      (await api.put(`/agents/${agentId}/api/webhook`, { webhook_url })).data as { webhook_secret?: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useRotateWebhookSecret(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/api/webhook-secret`)).data as { webhook_secret: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useTestWebhook(agentId: number) {
  return useMutation({
    mutationFn: async () =>
      (await api.post(`/agents/${agentId}/api/webhook/test`)).data as { status: string; http_status: number },
  });
}

/** Uji kirim lewat jalur REST API (JWT dashboard, tanpa mengekspos API key). */
export function useTestApiMessage(agentId: number) {
  return useMutation({
    mutationFn: async (payload: { to: string; text?: string }) =>
      (await api.post(`/agents/${agentId}/api/test-message`, payload)).data as {
        status: string;
        to: string;
        type: string;
        message_id?: string;
        note?: string;
      },
  });
}

export function useFlow(agentId: number) {
  return useQuery<Flow>({
    queryKey: ['flow', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/flow`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveFlow(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (f: Partial<Flow>) => (await api.post(`/agents/${agentId}/flow`, f)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['flow', agentId] }),
  });
}

// ---- Status / Story ----

export function useStatuses(agentId: number) {
  return useQuery<ScheduledStatus[]>({
    queryKey: ['statuses', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/statuses`)).data.data,
    enabled: !!agentId,
  });
}

export function useCreateStatus(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fd: FormData) => (await api.post(`/agents/${agentId}/status`, fd)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['statuses', agentId] }),
  });
}

export function useCancelStatus(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sid: number) => (await api.delete(`/agents/${agentId}/status/${sid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['statuses', agentId] }),
  });
}

// ---- Pairing ----

export function useAgentConnectPairing(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (phone: string) => (await api.post(`/agents/${agentId}/wa/connect-pairing`, { phone })).data as { status: string },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'status'] });
      qc.invalidateQueries({ queryKey: ['agent-statuses'] });
    },
  });
}

// ---- Usage ----

export function useUsage() {
  return useQuery<{ tenant: { id: number; name: string }; numbers_used: number; max_numbers: number }>({
    queryKey: ['usage'],
    queryFn: async () => (await api.get('/usage')).data,
  });
}

// Media assets (SEND_MEDIA directive).
export function useMediaAssets(agentId: number) {
  return useQuery<MediaAsset[]>({
    queryKey: ['media-assets', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/media-assets`)).data.data,
    enabled: !!agentId,
  });
}

export function useUploadMediaAsset(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fd: FormData) =>
      (await api.post(`/agents/${agentId}/media-assets`, fd)).data.data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['media-assets', agentId] });
    },
  });
}

export function useDeleteMediaAsset(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (assetId: number) =>
      (await api.delete(`/agents/${agentId}/media-assets/${assetId}`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['media-assets', agentId] });
    },
  });
}

// ---- Learning Engine hooks (single-tenant) ----

export function useLearningStatus(agentId: number) {
  return useQuery<LearningStatus>({
    queryKey: ['learning-status', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/status`)).data.data,
    enabled: !!agentId,
    refetchInterval: 5000,
  });
}

export function useLearningScore(agentId: number) {
  return useQuery<LearningScore>({
    queryKey: ['learning-score', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/score`)).data.data,
    enabled: !!agentId,
    refetchInterval: 30_000,
  });
}

export function useLearningRuns(agentId: number) {
  return useQuery<LearningRun[]>({
    queryKey: ['learning-runs', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/runs`)).data.data,
    enabled: !!agentId,
  });
}

export function useLearningRun(agentId: number, runId: number) {
  return useQuery<LearningRunDetail>({
    queryKey: ['learning-run', agentId, runId],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/runs/${runId}`)).data.data,
    enabled: !!agentId && !!runId,
  });
}

export function useLearningPatterns(agentId: number, status?: string) {
  return useQuery<LearningPattern[]>({
    queryKey: ['learning-patterns', agentId, status],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/patterns`, { params: status ? { status } : {} })).data.data,
    enabled: !!agentId,
  });
}

export function useLearningSnapshots(agentId: number) {
  return useQuery<LearningSnapshot[]>({
    queryKey: ['learning-snapshots', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/snapshots`)).data.data,
    enabled: !!agentId,
  });
}

export function useLearningConfig(agentId: number) {
  return useQuery<LearningConfig>({
    queryKey: ['learning-config', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/learning/config`)).data.data,
    enabled: !!agentId,
  });
}

export function useStartLearning(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { start_date?: string; end_date?: string }) =>
      (await api.post(`/agents/${agentId}/learning/run`, body)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['learning-runs', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-status', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-patterns', agentId] });
    },
  });
}

export function useApplyPattern(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patternId: number) =>
      (await api.post(`/agents/${agentId}/learning/patterns/${patternId}/apply`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['learning-patterns', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-status', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-score', agentId] });
    },
  });
}

export function useRejectPattern(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patternId: number) =>
      (await api.post(`/agents/${agentId}/learning/patterns/${patternId}/reject`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['learning-patterns', agentId] }),
  });
}

export function useApplyAllPatterns(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (minConfidence: number) =>
      (await api.post(`/agents/${agentId}/learning/patterns/apply-all`, { min_confidence: minConfidence })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['learning-patterns', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-status', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-score', agentId] });
    },
  });
}

export function useCreateSnapshot(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (label: string) =>
      (await api.post(`/agents/${agentId}/learning/snapshots`, { label })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['learning-snapshots', agentId] }),
  });
}

export function useRollbackSnapshot(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (snapshotId: number) =>
      (await api.post(`/agents/${agentId}/learning/snapshots/${snapshotId}/rollback`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['learning-snapshots', agentId] });
      qc.invalidateQueries({ queryKey: ['learning-status', agentId] });
    },
  });
}

export function useSaveLearningConfig(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Partial<LearningConfig>) =>
      (await api.put(`/agents/${agentId}/learning/config`, body)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['learning-config', agentId] }),
  });
}

// ---- Meta CAPI hooks (single-tenant) ----

export interface MetaConfigInput {
  pixel_id: string;
  access_token: string;
  test_event_code: string;
  conv_labels: string;
  event_name: string;
  label_events: Record<string, string>;
}

export function useMetaConfig(agentId: number) {
  return useQuery<MetaConfigData>({
    queryKey: ['meta-config', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/meta`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveMetaConfig(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: MetaConfigInput) =>
      (await api.put(`/agents/${agentId}/meta`, body)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['meta-config', agentId] }),
  });
}

export function useTestMetaEvent(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/meta/test`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['meta-config', agentId] }),
  });
}
