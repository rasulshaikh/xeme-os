// Server-side fetch helpers for the Xeme Ledger REST API.
const XEME_BASE = process.env.XEME_BASE_URL || 'http://localhost:8088';

export interface Contact {
  id: number; company_id: number; first_name: string; last_name: string;
  email: string; job_title: string; linkedin_url: string; score: number;
  tier: string; source: string; created_at: string; updated_at: string;
}
export interface Company { id: number; domain: string; name: string; industry: string; size: string; created_at: string; }
export interface Stats {
  contacts: number; companies: number; deals: number; outcomes: number;
  by_tier: Record<string, number>; by_signal: Record<string, number>;
}
export interface Workflow { id: string; name: string; description?: string; nodes: any[]; created_at: string; }
export interface WorkflowDetail extends Workflow { runs: any[]; }
export interface Campaign { id: string; name: string; template_id: string; status: string; steps: any[]; created_at: string; }
export interface CampaignDetail extends Campaign { contacts: any[]; events: any[]; }
export interface CampaignEvent { id: number; email: string; step_id: string; channel: string; event: string; payload: string; at: string; }

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${XEME_BASE}${path}`, { cache: 'no-store' });
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json();
}

export const api = {
  health: () => get<{ ok: boolean; engine: string }>('/health'),
  stats: () => get<Stats>('/v1/stats'),
  contacts: (params?: { q?: string; min_score?: number; tier?: string; limit?: number }) => {
    const qs = new URLSearchParams();
    if (params?.q) qs.set('q', params.q);
    if (params?.min_score !== undefined) qs.set('min_score', String(params.min_score));
    if (params?.tier) qs.set('tier', params.tier);
    if (params?.limit) qs.set('limit', String(params.limit));
    return get<Contact[]>(`/v1/contacts?${qs}`);
  },
  contact: (id: number) => get<Contact>(`/v1/contacts/${id}`),
  companies: () => get<Company[]>('/v1/companies'),
  workflows: () => get<Workflow[]>('/v1/workflows'),
  workflow: (id: string) => get<WorkflowDetail>(`/v1/workflows/${id}`),
  campaigns: () => get<Campaign[]>('/v1/campaigns'),
  campaign: (id: string) => get<CampaignDetail>(`/v1/campaigns/${id}`),
  campaignEvents: (id: string) => get<CampaignEvent[]>(`/v1/campaigns/${id}/events`),
};

// Auth helpers
export const ADMIN_USER = process.env.XEME_ADMIN_USER || '';
export const ADMIN_PASS = process.env.XEME_ADMIN_PASS || '';
export function authEnabled(): boolean { return ADMIN_USER.length > 0 && ADMIN_PASS.length > 0; }
