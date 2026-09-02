// inboxDebug.ts — laporan kondisi realtime klien ke server (untuk diagnosa).
import api from './api';

export async function reportInboxDebug(agentId: number, payload: Record<string, unknown>) {
  try {
    await api.post(`/agents/${agentId}/inbox/client-debug`, payload);
  } catch {
    // Debug tidak boleh mengganggu alur utama.
  }
}
