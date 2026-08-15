import { getToken, logout as authLogout } from "./auth";

const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export interface TokenResponse {
  token: string;
  expires_in: number;
  user_id: string;
  tenant_id: string;
  is_super_admin?: boolean;
}

export interface InstallDefaults {
  llm: {
    name: string;
    provider: string;
    endpoint: string;
    model: string;
    temperature: number;
    max_tokens: number;
    top_p: number;
  };
  embedding: {
    name: string;
    endpoint: string;
    model: string;
    dim: number;
    batch_size: number;
  };
  rerank: {
    name: string;
    endpoint: string;
    model: string;
    top_n: number;
  };
}

export interface InstallInput {
  email: string;
  password: string;
  name: string;
  tenant_name: string;
  provider_key: string;
}

export interface Paginated<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

export interface QuotaItem {
  used: number;
  limit: number;
  warning: boolean;
}

// A tenant row as seen by the super admin (all tenants, with usage stats).
export interface AdminTenant {
  id: string;
  name: string;
  plan: string;
  doc_quota: number;
  vector_quota: number;
  message_quota: number;
  user_message_quota: number;
  member_count: number;
  doc_used: number;
  msg_used: number;
  created_at: string;
}

export interface KnowledgeBase {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  embedding_model_id: string;
  chunk_strategy: string;
  chunk_size: number;
  chunk_overlap: number;
  doc_count: number;
  owner_id: string;
  visibility: string;
  created_at: string;
  updated_at: string;
}

export interface Document {
  id: string;
  tenant_id: string;
  kb_id: string;
  name: string;
  size: number;
  mime_type: string;
  object_key: string;
  parsed_object_key: string;
  status: string;
  parse_error?: string;
  enabled: boolean;
  chunk_count: number;
  owner_id: string;
  source_url?: string;
  metadata?: string;
  // Summed chunk hit_num (computed at read time, not persisted).
  hit_count?: number;
  created_at: string;
  updated_at: string;
}

export interface PreviewChunk {
  index: number;
  content: string;
  token_count: number;
}

export interface PreviewResult {
  strategy: string;
  total: number;
  token_estimate: number;
  parent_count?: number;
  chunks: PreviewChunk[];
}

export interface Annotation {
  id: string;
  tenant_id: string;
  kb_id: string;
  question: string;
  answer: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Chunk {
  id: string;
  tenant_id: string;
  kb_id: string;
  doc_id: string;
  index: number;
  content: string;
  token_count: number;
  page_numbers: string;
  vector_id: string;
  created_at: string;
}

export interface DocContent {
  name: string;
  mime_type: string;
  status: string;
  chunk_count: number;
  content: string;
}

export interface LLMModel {
  id: string;
  tenant_id: string;
  name: string;
  provider: string;
  endpoint: string;
  api_key?: string;
  model: string;
  temperature: number;
  max_tokens: number;
  top_p: number;
  context_length: number;
  is_default: boolean;
  owner_id: string;
  status: string;
  last_tested_at: string | null;
  last_test_status: string;
  created_at: string;
  updated_at: string;
}

// Candidate from an OpenAI-compatible GET /models listing. context_length /
// max_tokens are present only when the endpoint reports them.
export interface DiscoveredModel {
  id: string;
  name?: string;
  context_length?: number;
  max_tokens?: number;
}

// Static catalog entry: a known vendor with a default endpoint and curated
// models, so adding the provider needs only an API key.
export interface CatalogProvider {
  id: string;
  name: string;
  endpoint: string;
  note?: string;
  chat_models: { id: string; context_length?: number; note?: string }[];
  embedding_models: { id: string; context_length?: number; note?: string }[];
  rerank_models: { id: string; context_length?: number; note?: string }[];
}

// One model row bound to a provider card.
export interface ProviderModelRef {
  id: string;
  name: string;
  model: string;
  context_length?: number;
  is_default: boolean;
  last_test_status: string;
}

// Provider card: endpoint + one write-only credential + bound models for each kind.
export interface ProviderCard {
  id: string;
  tenant_id: string;
  catalog_id: string;
  name: string;
  endpoint: string;
  has_key: boolean;
  created_at: string;
  updated_at: string;
  chat_models: ProviderModelRef[];
  embed_models: ProviderModelRef[];
  rerank_models: ProviderModelRef[];
}

export interface EmbeddingModel {
  id: string;
  tenant_id: string;
  provider_id?: string;
  name: string;
  endpoint: string;
  api_key?: string;
  model: string;
  dim: number;
  batch_size: number;
  is_default: boolean;
  owner_id: string;
  status: string;
  last_tested_at: string | null;
  last_test_status: string;
  created_at: string;
  updated_at: string;
}

export interface RerankModel {
  id: string;
  tenant_id: string;
  provider_id?: string;
  name: string;
  endpoint: string;
  api_key?: string;
  model: string;
  top_n: number;
  is_default: boolean;
  owner_id: string;
  status: string;
  last_tested_at: string | null;
  last_test_status: string;
  created_at: string;
  updated_at: string;
}

export interface Conversation {
  id: string;
  tenant_id: string;
  kb_id: string;
  title: string;
  owner_id: string;
  pinned: boolean;
  created_at: string;
  updated_at: string;
}

export interface UserProfile {
  id: string;
  email: string;
  name: string;
  language: string;
  role: string;
  status: string;
  is_super_admin: boolean;
  tenant_name: string;
  created_at: string;
}

export interface Message {
  id: string;
  tenant_id: string;
  conversation_id: string;
  role: string;
  content: string;
  reasoning?: string;
  citations?: string;
  retrieve_ms?: number;
  generate_ms?: number;
  total_ms?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  // When true, this assistant message came from a matched annotation
  // reply rather than LLM generation. Persisted server-side so the
  // "标注回复" label survives page reloads.
  annotation?: boolean;
  // LLM-generated follow-up suggestions for this assistant message, stored
  // as a JSON string array; rendered as clickable chips under the reply.
  follow_ups?: string;
  // User feedback on an assistant message: "up", "down", or unset.
  vote?: string;
  created_at: string;
}

export interface ReplyStats {
  retrieve_ms: number;
  generate_ms: number;
  total_ms: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface Citation {
  chunk_id: string;
  doc_id: string;
  doc_name: string;
  score?: number;
  page_numbers?: string;
  content?: string;
}

export interface SearchHit {
  chunk_id: string;
  doc_id: string;
  doc_name: string;
  content: string;
  score: number;
  page_numbers?: string;
}

export interface SearchResult {
  hits: SearchHit[];
  sparse: boolean;
  rerank: boolean;
  graph_context?: string;
  debug_dense_hits?: SearchHit[];
  debug_sparse_hits?: SearchHit[];
  debug_fused_scores?: Record<string, number>;
}

export interface TeamUser {
  id: string;
  email: string;
  name: string;
  role: "admin" | "member";
  status: "active" | "disabled";
  created_at: string;
  message_used: number;
}

// System-wide user (super admin user management)
export interface SystemUser {
  id: string;
  email: string;
  name: string;
  role: "admin" | "member";
  status: "active" | "disabled";
  is_super_admin: boolean;
  tenant_name: string;
  created_at: string;
}

// Tenant invitations
export interface Invitation {
  id: string;
  email: string;
  role: "admin" | "member";
  status: "pending" | "accepted" | "expired" | "cancelled";
  expires_at: string;
  created_at: string;
}

// InvitationView is the public payload returned by GET /invitations/:token.
export interface InvitationView {
  token: string;
  email: string;
  role: "admin" | "member";
  tenant_name?: string;
  status: string;
  expires_at: string;
}

// AcceptInvitationResponse is returned after accepting an invitation.
export interface AcceptInvitationResponse {
  user_id: string;
  tenant_id: string;
  email: string;
}

// Ingestion Pipeline canvas
export interface PipelineNode {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: Record<string, unknown>;
}
export interface PipelineEdge {
  id: string;
  source: string;
  target: string;
}
export interface PipelineDefinition {
  nodes: PipelineNode[];
  edges: PipelineEdge[];
}
export interface Pipeline {
  id: string;
  kb_id: string;
  version: number;
  definition: PipelineDefinition;
  active: boolean;
  updated_at: string;
}

// Agent canvas
export interface AgentNode {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: Record<string, unknown>;
}
export interface AgentEdge {
  id: string;
  source: string;
  target: string;
  label?: string;
}
export interface AgentDefinition {
  nodes: AgentNode[];
  edges: AgentEdge[];
  opening_message?: string;
  suggested_questions?: string[];
}
export interface Agent {
  id: string;
  kb_id: string;
  name: string;
  version: number;
  definition: AgentDefinition;
  active: boolean;
  updated_at: string;
}

// Memory
export interface Memory {
  id: string;
  kb_id: string;
  conversation_id: string;
  title: string;
  summary: string;
  key_points?: string;
  created_at: string;
  updated_at: string;
}

// API Keys
export interface APIKey {
  id: string;
  tenant_id: string;
  name: string;
  key_prefix: string;
  status: "active" | "revoked";
  last_used_at?: string;
  expires_at?: string;
  created_at: string;
}

export interface APIKeyWithSecret extends APIKey {
  full_key: string; // only returned on creation
}

// Analytics
export interface AnalyticsOverview {
  knowledge_bases: number;
  documents: number;
  chunks: number;
  conversations: number;
  messages: number;
  storage_bytes: number;
}

export interface DocStatusCount {
  status: string;
  count: number;
}

export interface AnalyticsDocStats {
  total: number;
  by_status: DocStatusCount[];
  success_rate: number;
  total_chunks: number;
}

export interface KBUsage {
  kb_id: string;
  kb_name: string;
  doc_count: number;
  chunk_count: number;
  storage_bytes: number;
  conversations: number;
}

export interface ActivityItem {
  kind: string;
  id: string;
  name: string;
  status?: string;
  kb_id: string;
  created_at: string;
}

// Voted assistant message for bad-case review.
export interface FeedbackItem {
  id: string;
  conversation_id: string;
  kb_id: string;
  kb_name: string;
  user_name: string;
  question: string;
  answer: string;
  vote: string; // "up" | "down"
  created_at: string;
}

// Token usage & cost (bill rows written per LLM call). amount is an
// estimated cost in yuan based on each model's per-1M-token prices.
export interface BillOverview {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  amount: number;
  call_count: number;
}

export interface UserBillTotal {
  user_id: string;
  user_name: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  amount: number;
  call_count: number;
}

export interface ModelBillTotal {
  model_id: string;
  model_name: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  amount: number;
  call_count: number;
}

export interface BillRecord {
  id: string;
  tenant_id: string;
  user_id: string;
  kb_id?: string;
  conversation_id?: string;
  source: string; // chat | classifier | intermediate | followups
  provider: string;
  model_id?: string;
  model_name: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  amount: number;
  created_at: string;
}

// Audit
export interface AuditLog {
  id: string;
  tenant_id: string;
  user_id: string;
  user_name: string;
  action: string;
  resource: string;
  detail: string;
  ip: string;
  created_at: string;
}

// DocEvent is the payload pushed by the /documents/events SSE stream to
// notify clients of document status transitions in real time.
export interface DocEvent {
  tenant_id: string;
  doc_id: string;
  kb_id: string;
  status: string;
  error?: string;
  at: string;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers || {}),
    },
  });
  // 401 with a token means the token is invalid or expired; clear it and redirect.
  // 401 without a token (e.g. login with wrong password) is a normal auth error.
  if (res.status === 401) {
    if (token) {
      authLogout();
      throw new ApiError("session expired", 401);
    }
    const errJson = await res.json().catch(() => ({}));
    throw new ApiError(errJson.message || "invalid credentials", 401);
  }
  // 204 No Content: success with no body (e.g. DELETE).
  if (res.status === 204) {
    return undefined as T;
  }
  const json = await res.json().catch(() => ({}));
  if (!res.ok || json.code !== 0) {
    throw new ApiError(json.message || `request failed: ${res.status}`, res.status);
  }
  return json.data as T;
}

export const api = {
  // Install
  async installStatus(): Promise<{ installed: boolean }> {
    return request("/install/status");
  },
  async installDefaults(): Promise<InstallDefaults> {
    return request("/install/defaults");
  },
  async install(body: InstallInput): Promise<TokenResponse> {
    return request<TokenResponse>("/install", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  // Auth
  async login(body: { email: string; password: string }): Promise<TokenResponse> {
    return request<TokenResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async register(body: {
    email: string;
    password: string;
    name?: string;
    tenant_name?: string;
  }): Promise<TokenResponse> {
    return request<TokenResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async me(): Promise<{ user_id: string; tenant_id: string; role: string }> {
    return request("/auth/me");
  },
  async getProfile(): Promise<UserProfile> {
    return request("/user/profile");
  },
  async updateProfile(name: string, language: string): Promise<void> {
    await request("/user/profile", { method: "PUT", body: JSON.stringify({ name, language }) });
  },
  async changePassword(oldPassword: string, newPassword: string): Promise<void> {
    await request("/user/password", {
      method: "PUT",
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    });
  },
  async logout(): Promise<void> {
    try {
      await request("/auth/logout", { method: "POST" });
    } catch {
      // Network or 401: still clear local token below.
    }
    authLogout();
  },

  // Knowledge bases
  async listKBs(page = 1, size = 20): Promise<Paginated<KnowledgeBase>> {
    return request(`/knowledge-bases/?page=${page}&size=${size}`);
  },
  async createKB(body: {
    name: string;
    description?: string;
    embedding_model_id?: string;
    chunk_size?: number;
    chunk_overlap?: number;
  }): Promise<KnowledgeBase> {
    return request("/knowledge-bases/", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async getKB(id: string): Promise<KnowledgeBase> {
    return request(`/knowledge-bases/${id}`);
  },
  async updateKB(id: string, body: { name?: string; description?: string; embedding_model_id?: string }): Promise<KnowledgeBase> {
    return request(`/knowledge-bases/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },
  async deleteKB(id: string): Promise<void> {
    await request(`/knowledge-bases/${id}`, { method: "DELETE" });
  },
  async setKBVisibility(id: string, visibility: "private" | "team"): Promise<KnowledgeBase> {
    return request(`/knowledge-bases/${id}/visibility`, {
      method: "PUT",
      body: JSON.stringify({ visibility }),
    });
  },

  // Pipeline canvas
  async getPipeline(kbId: string): Promise<Pipeline> {
    return request(`/knowledge-bases/${kbId}/pipeline`);
  },
  async savePipeline(kbId: string, def: PipelineDefinition): Promise<Pipeline> {
    return request(`/knowledge-bases/${kbId}/pipeline`, {
      method: "PUT",
      body: JSON.stringify(def),
    });
  },

  // Agent canvas
  async getAgent(kbId: string): Promise<Agent> {
    return request(`/knowledge-bases/${kbId}/agent`);
  },
  async saveAgent(kbId: string, def: AgentDefinition): Promise<Agent> {
    return request(`/knowledge-bases/${kbId}/agent`, {
      method: "PUT",
      body: JSON.stringify(def),
    });
  },

  // Agent execution history (replayable graph runs)
  async listExecutions(kbId: string, page = 1, size = 20, source?: string): Promise<Paginated<Execution>> {
    const q = source ? `&source=${source}` : "";
    return request(`/knowledge-bases/${kbId}/executions?page=${page}&size=${size}${q}`);
  },
  async getExecution(id: string): Promise<Execution> {
    return request(`/executions/${id}`);
  },
  // Find the execution that produced one assistant message; used by the
  // analytics feedback list to deep-link a bad case into replay.
  async getExecutionByMessage(kbId: string, messageId: string): Promise<Execution> {
    return request(`/knowledge-bases/${kbId}/executions/by-message/${messageId}`);
  },

  // Memory
  async summarizeConversation(convId: string): Promise<Memory> {
    return request(`/conversations/${convId}/summarize`, { method: "POST" });
  },
  async listMemories(kbId: string, page = 1, size = 20): Promise<Paginated<Memory>> {
    return request(`/knowledge-bases/${kbId}/memories?page=${page}&size=${size}`);
  },
  async deleteMemory(kbId: string, id: string): Promise<void> {
    await request(`/knowledge-bases/${kbId}/memories/${id}`, { method: "DELETE" });
  },

  // Documents
  async listDocs(kbId: string, page = 1, size = 20): Promise<Paginated<Document>> {
    return request(`/knowledge-bases/${kbId}/documents/?page=${page}&size=${size}`);
  },
  async uploadDoc(kbId: string, file: File, onProgress?: (pct: number) => void): Promise<Document> {
    const token = getToken();
    const form = new FormData();
    form.append("file", file);
    const url = `${API_BASE}/api/v1/knowledge-bases/${kbId}/documents/`;
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", url);
      xhr.withCredentials = true;
      if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      };
      xhr.onload = () => {
        try {
          const json = JSON.parse(xhr.responseText);
          if (xhr.status >= 200 && xhr.status < 300 && json.code === 0) {
            resolve(json.data as Document);
          } else {
            reject(new ApiError(json.message || `upload failed: ${xhr.status}`, xhr.status));
          }
        } catch {
          reject(new ApiError(`upload failed: ${xhr.status}`, xhr.status));
        }
      };
      xhr.onerror = () => reject(new ApiError("upload network error", 0));
      xhr.send(form);
    });
  },
  async reparseDoc(kbId: string, docId: string): Promise<void> {
    await request(`/knowledge-bases/${kbId}/documents/${docId}/reparse`, {
      method: "POST",
    });
  },

  async setDocEnabled(kbId: string, docId: string, enabled: boolean): Promise<void> {
    await request(`/knowledge-bases/${kbId}/documents/${docId}/enabled`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    });
  },
  async deleteDoc(kbId: string, docId: string): Promise<void> {
    await request(`/knowledge-bases/${kbId}/documents/${docId}`, {
      method: "DELETE",
    });
  },
  async listChunks(kbId: string, docId: string): Promise<{ items: Chunk[] }> {
    return request(`/knowledge-bases/${kbId}/documents/${docId}/chunks`);
  },
  async getDocContent(kbId: string, docId: string): Promise<DocContent> {
    return request(`/knowledge-bases/${kbId}/documents/${docId}/content`);
  },
  async updateChunk(kbId: string, docId: string, chunkId: string, content: string): Promise<Chunk> {
    return request(`/knowledge-bases/${kbId}/documents/${docId}/chunks/${chunkId}`, {
      method: "PUT",
      body: JSON.stringify({ content }),
    });
  },
  async deleteChunk(kbId: string, docId: string, chunkId: string): Promise<void> {
    await request(`/knowledge-bases/${kbId}/documents/${docId}/chunks/${chunkId}`, {
      method: "DELETE",
    });
  },
  async previewChunks(
    kbId: string,
    body: { text: string; strategy?: string; size?: number; overlap?: number; is_csv?: boolean }
  ): Promise<PreviewResult> {
    return request(`/knowledge-bases/${kbId}/documents/preview`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async updateDocMetadata(kbId: string, docId: string, metadata: Record<string, string>): Promise<Document> {
    return request(`/knowledge-bases/${kbId}/documents/${docId}/metadata`, {
      method: "PUT",
      body: JSON.stringify({ metadata }),
    });
  },
  async batchDeleteDocs(kbId: string, ids: string[]): Promise<{ deleted: number }> {
    return request(`/knowledge-bases/${kbId}/documents/batch-delete`, {
      method: "POST",
      body: JSON.stringify({ ids }),
    });
  },
  async batchReparseDocs(kbId: string, ids: string[]): Promise<{ queued: number }> {
    return request(`/knowledge-bases/${kbId}/documents/batch-reparse`, {
      method: "POST",
      body: JSON.stringify({ ids }),
    });
  },
  async importURL(kbId: string, url: string): Promise<Document> {
    return request(`/knowledge-bases/${kbId}/documents/import-url`, {
      method: "POST",
      body: JSON.stringify({ url }),
    });
  },

  // Annotations
  async listAnnotations(kbId: string, page = 1, size = 50): Promise<Paginated<Annotation>> {
    return request(`/knowledge-bases/${kbId}/annotations/?page=${page}&size=${size}`);
  },
  async createAnnotation(
    kbId: string,
    body: { question: string; answer: string; enabled?: boolean }
  ): Promise<Annotation> {
    return request(`/knowledge-bases/${kbId}/annotations/`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async updateAnnotation(
    kbId: string,
    id: string,
    body: { question?: string; answer?: string; enabled?: boolean }
  ): Promise<Annotation> {
    return request(`/knowledge-bases/${kbId}/annotations/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },
  async deleteAnnotation(kbId: string, id: string): Promise<void> {
    await request(`/knowledge-bases/${kbId}/annotations/${id}`, {
      method: "DELETE",
    });
  },

  // LLM providers
  async listLLMs(page = 1, size = 20): Promise<Paginated<LLMModel>> {
    return request(`/llm-models/?page=${page}&size=${size}`);
  },
  async createLLM(body: Partial<LLMModel>): Promise<LLMModel> {
    return request("/llm-models/", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async updateLLM(id: string, body: Record<string, unknown>): Promise<LLMModel> {
    return request(`/llm-models/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },
  async deleteLLM(id: string): Promise<void> {
    await request(`/llm-models/${id}`, { method: "DELETE" });
  },
  async testLLM(id: string): Promise<{ reply: string }> {
    return request(`/llm-models/${id}/test`, { method: "POST" });
  },

  // Embedding providers
  async listEmbeddings(page = 1, size = 50): Promise<Paginated<EmbeddingModel>> {
    return request(`/embedding-models/?page=${page}&size=${size}`);
  },
  async createEmbedding(body: Partial<EmbeddingModel>): Promise<EmbeddingModel> {
    return request("/embedding-models/", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async updateEmbedding(id: string, body: Record<string, unknown>): Promise<EmbeddingModel> {
    return request(`/embedding-models/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },
  async deleteEmbedding(id: string): Promise<void> {
    await request(`/embedding-models/${id}`, { method: "DELETE" });
  },
  async testEmbedding(id: string): Promise<{ reply: string }> {
    return request(`/embedding-models/${id}/test`, { method: "POST" });
  },

  // Rerank models
  async listReranks(page = 1, size = 50): Promise<Paginated<RerankModel>> {
    return request(`/rerank-models/?page=${page}&size=${size}`);
  },
  async createRerank(body: Partial<RerankModel>): Promise<RerankModel> {
    return request("/rerank-models/", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async updateRerank(id: string, body: Record<string, unknown>): Promise<RerankModel> {
    return request(`/rerank-models/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },
  async deleteRerank(id: string): Promise<void> {
    await request(`/rerank-models/${id}`, { method: "DELETE" });
  },
  async testRerank(id: string): Promise<{ reply: string }> {
    return request(`/rerank-models/${id}/test`, { method: "POST" });
  },

  // Provider cards group model rows under one endpoint+credential
  // (settings page). kind picks the route group.
  providers: {
    async list(): Promise<ProviderCard[]> {
      const r = await request<{ items: ProviderCard[] }>("/providers");
      return r.items ?? [];
    },
    async catalog(): Promise<CatalogProvider[]> {
      const r = await request<{ items: CatalogProvider[] }>("/providers/catalog");
      return r.items ?? [];
    },
    async create(body: {
      catalog_id: string;
      name?: string;
      endpoint?: string;
      api_key?: string;
      chat_models?: { model: string; name?: string; context_length?: number }[];
      embed_models?: { model: string; name?: string; context_length?: number }[];
      rerank_models?: { model: string; name?: string; context_length?: number }[];
    }): Promise<ProviderCard> {
      return request("/providers", { method: "POST", body: JSON.stringify(body) });
    },
    async update(
      id: string,
      body: { name?: string; endpoint?: string; api_key?: string }
    ): Promise<ProviderCard> {
      return request(`/providers/${id}`, { method: "PUT", body: JSON.stringify(body) });
    },
    async remove(id: string): Promise<void> {
      await request(`/providers/${id}`, { method: "DELETE" });
    },
    async discover(id: string): Promise<DiscoveredModel[]> {
      const r = await request<{ items: DiscoveredModel[] }>(`/providers/${id}/discover`, {
        method: "POST",
      });
      return r.items ?? [];
    },
    // Probe asks an arbitrary endpoint+key (what the form currently shows)
    // for its model list, before the provider exists or is saved.
    async probe(body: {
      endpoint: string;
      api_key?: string;
      provider_id?: string;
    }): Promise<DiscoveredModel[]> {
      const r = await request<{ items: DiscoveredModel[] }>("/providers/probe", {
        method: "POST",
        body: JSON.stringify(body),
      });
      return r.items ?? [];
    },
    // ProbeModel tests a single unsaved model (form endpoint+key) for
    // connectivity, so the settings drawer can validate a model before saving.
    async probeModel(body: {
      kind: string;
      endpoint: string;
      api_key: string;
      model: string;
    }): Promise<{ reply: string }> {
      return request("/providers/probe-model", {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
    async addModel(
      id: string,
      kind: "llm" | "embedding" | "rerank",
      body: { model: string; name?: string; context_length?: number }
    ): Promise<ProviderModelRef> {
      return request(`/providers/${id}/models/${kind}`, {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
    async updateModel(
      id: string,
      kind: "llm" | "embedding" | "rerank",
      mid: string,
      body: { name?: string; context_length?: number }
    ): Promise<ProviderModelRef> {
      return request(`/providers/${id}/models/${kind}/${mid}`, {
        method: "PUT",
        body: JSON.stringify(body),
      });
    },
    async removeModel(
      id: string,
      kind: "llm" | "embedding" | "rerank",
      mid: string
    ): Promise<void> {
      await request(`/providers/${id}/models/${kind}/${mid}`, { method: "DELETE" });
    },
  },

  // Search
  async search(kbId: string, body: {
    query: string;
    top_k?: number;
    rerank?: boolean;
    rerank_model_id?: string;
    vector_weight?: number;
  }): Promise<SearchResult> {
    return request(`/knowledge-bases/${kbId}/search`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  async searchDebug(kbId: string, body: {
    query: string;
    top_k?: number;
    rerank?: boolean;
    rerank_model_id?: string;
    vector_weight?: number;
  }): Promise<SearchResult> {
    return request(`/knowledge-bases/${kbId}/search/debug`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  // Conversations / messages
  async listConversations(page = 1, size = 50, query?: string): Promise<Paginated<Conversation>> {
    const q = query ? `&q=${encodeURIComponent(query)}` : "";
    return request(`/conversations?page=${page}&size=${size}${q}`);
  },
  async createConversation(kbId: string, body: { title?: string }): Promise<Conversation> {
    return request(`/knowledge-bases/${kbId}/conversations`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  async renameConversation(convId: string, title: string): Promise<Conversation> {
    return request(`/conversations/${convId}`, {
      method: "PUT",
      body: JSON.stringify({ title }),
    });
  },
  async pinConversation(convId: string, pinned: boolean): Promise<Conversation> {
    return request(`/conversations/${convId}/pin`, {
      method: "PUT",
      body: JSON.stringify({ pinned }),
    });
  },
  async exportConversation(convId: string): Promise<void> {
    const token = getToken();
    const res = await fetch(`${API_BASE}/api/v1/conversations/${convId}/export`, {
      credentials: "include",
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    });
    if (!res.ok) throw new ApiError(`export failed: ${res.status}`, res.status);
    const blob = await res.blob();
    const cd = res.headers.get("Content-Disposition") || "";
    const m = cd.match(/filename="(.+?)"/);
    const filename = m ? m[1] : "conversation.md";
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  },
  async listMessages(convId: string): Promise<{ items: Message[] }> {
    return request(`/conversations/${convId}/messages`);
  },
  // Vote on an assistant message ("up"/"down"; sending the current value
  // again clears the vote).
  async voteMessage(convId: string, msgId: string, vote: "up" | "down" | ""): Promise<void> {
    await request(`/conversations/${convId}/messages/${msgId}/vote`, {
      method: "POST",
      body: JSON.stringify({ vote }),
    });
  },
  async deleteConversation(convId: string): Promise<void> {
    await request(`/conversations/${convId}`, { method: "DELETE" });
  },

  // Chat SSE stream
  // Returns an async generator that yields parsed StreamReply events. The
  // caller is responsible for rendering tokens as they arrive.
  streamChat(
    convId: string,
    body: { message: string; query?: string; top_k?: number },
    signal?: AbortSignal
  ): AsyncGenerator<StreamReply> {
    return streamSSE(
      `/conversations/${convId}/messages/stream`,
      body,
      signal
    ) as AsyncGenerator<StreamReply>;
  },

  // subscribeConversation opens a live SSE connection to follow a
  // conversation from another tab/device (observer stream). Yields the same
  // StreamReply events the sending client sees, plus "user" events carrying
  // questions asked elsewhere. Stays open until aborted.
  subscribeConversation(convId: string, signal?: AbortSignal): AsyncGenerator<StreamReply> {
    return streamGETSSE(`/conversations/${convId}/stream`, signal) as AsyncGenerator<StreamReply>;
  },

  // testChat streams an agent reply without persisting anything. Used by the
  // agent config test drawer. Same SSE format as streamChat, but no
  // conversation is created and no messages are saved.
  testChat(
    kbId: string,
    body: { message: string; query?: string },
    signal?: AbortSignal
  ): AsyncGenerator<StreamReply> {
    return streamSSE(
      `/knowledge-bases/${kbId}/test-chat`,
      body,
      signal
    ) as AsyncGenerator<StreamReply>;
  },

  // debugAgentNode runs one agent node in isolation (canvas "test this
  // node"). Returns the node's output without walking the whole graph.
  async debugAgentNode(
    kbId: string,
    node: AgentNode,
    query: string
  ): Promise<NodeDebugResult> {
    return request(`/knowledge-bases/${kbId}/debug-node`, {
      method: "POST",
      body: JSON.stringify({ node, query }),
    });
  },

  // Message quota: remaining is the effective daily remaining (min of tenant
  // and user remaining); quota is the user's personal limit (-1 = unlimited).
  async getMessageQuota(): Promise<{ remaining: number; quota: number }> {
    return request("/quota/messages");
  },

  // Document SSE events
  // streamDocEvents opens a GET Server-Sent Events connection for live
  // document status updates. Uses fetch streaming because the JWT is sent
  // via the Authorization header, which the native EventSource API cannot
  // set. Pass an AbortSignal to close the connection on unmount.
  streamDocEvents(signal?: AbortSignal): AsyncGenerator<DocEvent> {
    return streamDocEvents(signal);
  },

  // Tenant users (team management)
  async listTeamUsers(): Promise<{ items: TeamUser[] }> {
    return request("/tenant/users");
  },
  async updateUserRole(userId: string, role: "admin" | "member"): Promise<void> {
    await request(`/tenant/users/${userId}/role`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    });
  },
  async updateUserStatus(userId: string, status: "active" | "disabled"): Promise<void> {
    await request(`/tenant/users/${userId}/status`, {
      method: "PUT",
      body: JSON.stringify({ status }),
    });
  },
  async resetUserPassword(userId: string, newPassword: string): Promise<void> {
    await request(`/tenant/users/${userId}/reset-password`, {
      method: "POST",
      body: JSON.stringify({ new_password: newPassword }),
    });
  },

  // System user management (super admin)
  async listSystemUsers(): Promise<{ items: SystemUser[] }> {
    return request("/admin/users");
  },
  async updateUserSuperAdmin(userId: string, isSuperAdmin: boolean): Promise<void> {
    await request(`/admin/users/${userId}/super-admin`, {
      method: "PUT",
      body: JSON.stringify({ is_super_admin: isSuperAdmin }),
    });
  },

  // Tenant invitations
  async listInvitations(): Promise<{ items: Invitation[] }> {
    return request("/tenant/invitations");
  },
  async createInvitation(email: string, role: "admin" | "member"): Promise<{ invitation: Invitation; token: string }> {
    return request("/tenant/invitations", {
      method: "POST",
      body: JSON.stringify({ email, role }),
    });
  },
  async cancelInvitation(id: string): Promise<void> {
    await request(`/tenant/invitations/${id}`, { method: "DELETE" });
  },
  // peekInvitation is public: fetches invitation details by token without auth.
  async peekInvitation(token: string): Promise<InvitationView> {
    return request(`/invitations/${token}`);
  },
  async acceptInvitation(token: string, name: string, password: string): Promise<AcceptInvitationResponse> {
    return request("/invitations/accept", {
      method: "POST",
      body: JSON.stringify({ token, name, password }),
    });
  },

  // Tenant quota (admin view). user_message_limit is the per-user daily cap.
  async getQuota(): Promise<{ name: string; plan: string; member_count: number; doc: QuotaItem; vector: QuotaItem; message: QuotaItem; user_message_limit: number }> {
    return request("/tenant/quota");
  },

  // Update tenant (team) name. Admin only.
  async updateTenantName(name: string): Promise<void> {
    await request("/tenant", { method: "PUT", body: JSON.stringify({ name }) });
  },

  // Site settings (public, non-sensitive).
  async getSiteSettings(): Promise<{
    site_name: string;
    site_description: string;
    default_language: string;
    timezone: string;
    allow_registration: string;
  }> {
    return request("/site/settings");
  },

  // All site settings (super admin only).
  async getAllSiteSettings(): Promise<Record<string, string>> {
    return request("/site/settings/all");
  },

  // Update site settings (super admin only).
  async updateSiteSettings(updates: Record<string, string>): Promise<void> {
    await request("/site/settings", { method: "PUT", body: JSON.stringify(updates) });
  },

  // Tenant management (super admin only).
  async listAdminTenants(): Promise<AdminTenant[]> {
    const res = await request<AdminTenant[] | Paginated<AdminTenant>>("/admin/tenants");
    return Array.isArray(res) ? res : (res?.items ?? []);
  },
  async updateTenantQuota(
    id: string,
    quota: { doc_quota: number; vector_quota: number; message_quota: number; user_message_quota: number }
  ): Promise<void> {
    await request(`/admin/tenants/${id}/quota`, { method: "PUT", body: JSON.stringify(quota) });
  },
  async updateTenantPlan(id: string, plan: string): Promise<void> {
    await request(`/admin/tenants/${id}/plan`, { method: "PUT", body: JSON.stringify({ plan }) });
  },

  // API Keys
  async listApiKeys(): Promise<APIKey[]> {
    const res = await request<APIKey[] | Paginated<APIKey>>("/api-keys");
    // Tolerate either a bare array or a paginated envelope from the backend.
    return Array.isArray(res) ? res : (res?.items ?? []);
  },
  async createApiKey(name: string, expires_at?: string): Promise<APIKeyWithSecret> {
    return request<APIKeyWithSecret>("/api-keys", {
      method: "POST",
      body: JSON.stringify({ name, expires_at }),
    });
  },
  async revokeApiKey(id: string): Promise<void> {
    await request(`/api-keys/${id}/revoke`, { method: "POST" });
  },
  async deleteApiKey(id: string): Promise<void> {
    await request(`/api-keys/${id}`, { method: "DELETE" });
  },

  // Analytics
  async analyticsOverview(): Promise<AnalyticsOverview> {
    return request("/analytics/overview");
  },
  async analyticsDocStats(): Promise<AnalyticsDocStats> {
    return request("/analytics/documents");
  },
  async analyticsUsage(): Promise<{ items: KBUsage[] }> {
    return request("/analytics/usage");
  },
  async analyticsActivity(limit = 20): Promise<{ items: ActivityItem[] }> {
    return request(`/analytics/activity?limit=${limit}`);
  },
  async analyticsFeedback(limit = 50): Promise<{ items: FeedbackItem[] }> {
    return request(`/analytics/feedback?limit=${limit}`);
  },

  // Token usage & cost (bill rows written per LLM call)
  async billOverview(): Promise<BillOverview> {
    return request("/bills/overview");
  },
  async billUsers(): Promise<{ items: UserBillTotal[] }> {
    return request("/bills/users");
  },
  async billModels(): Promise<{ items: ModelBillTotal[] }> {
    return request("/bills/models");
  },
  async billRecords(limit = 100): Promise<{ items: BillRecord[] }> {
    return request(`/bills/records?limit=${limit}`);
  },

  // Audit logs (admin)
  async listAuditLogs(page = 1, size = 50, filters?: { user_id?: string; action?: string }): Promise<Paginated<AuditLog>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    if (filters?.user_id) params.set("user_id", filters.user_id);
    if (filters?.action) params.set("action", filters.action);
    return request(`/tenant/audit-logs?${params}`);
  },

  // Backup / restore (admin)
  async exportKB(kbId: string): Promise<void> {
    const token = getToken();
    const res = await fetch(`${API_BASE}/api/v1/knowledge-bases/${kbId}/export`, {
      credentials: "include",
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    });
    if (!res.ok) throw new ApiError(`export failed: ${res.status}`, res.status);
    const blob = await res.blob();
    const cd = res.headers.get("Content-Disposition") || "";
    const m = cd.match(/filename="(.+?)"/);
    const filename = m ? m[1] : "kb-backup.json";
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  },
  async importKB(kbId: string, file: File): Promise<{ imported_documents: number }> {
    const token = getToken();
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`${API_BASE}/api/v1/knowledge-bases/${kbId}/import`, {
      method: "POST",
      credentials: "include",
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      body: form,
    });
    const json = await res.json().catch(() => ({}));
    if (!res.ok || json.code !== 0) {
      throw new ApiError(json.message || `import failed: ${res.status}`, res.status);
    }
    return json.data;
  },
};

export interface TraceStep {
  node_id?: string;
  node_type?: string;
  edge_id?: string;
  status?: string;
  duration_ms?: number;
  detail?: string;
}

// One persisted agent-graph run with its full node trace for replay.
export interface Execution {
  id: string;
  kb_id: string;
  conversation_id?: string;
  message_id?: string;
  user_id?: string;
  source: "chat" | "test";
  status: "success" | "error" | "cancelled";
  query: string;
  answer?: string;
  terminal_type?: string;
  trace: string;
  total_ms: number;
  created_at: string;
}

// Result of the single-node debug API (agent canvas "test this node").
export interface NodeDebugResult {
  type: string;
  text?: string;
  hits?: number;
  top_score?: number;
  context?: string;
  duration_ms?: number;
  /** Variables the executor would set after running this node. */
  variables?: Record<string, string>;
}

export interface StreamReply {
  // "user" appears on subscription streams only (a question asked on
  // another client); the sending client never receives it.
  phase: "retrieve" | "thinking" | "generate" | "done" | "follow_ups" | "error" | "warning" | "trace" | "user";
  token?: string;
  citations?: Citation[];
  message_id?: string;
  error?: string;
  warning?: string;
  stats?: ReplyStats;
  trace?: TraceStep[];
  annotation?: boolean;
  questions?: string[];
}

// streamSSE opens a POST request and parses the Server-Sent Events response.
// Used by chat streaming. The fetch ReadableStream is read incrementally so
// tokens appear in the UI as soon as the server emits them.
async function* streamSSE(path: string, body: unknown, signal?: AbortSignal): AsyncGenerator<unknown> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    const txt = await res.text().catch(() => "");
    throw new ApiError(txt || `stream failed: ${res.status}`, res.status);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    // SSE events are separated by a blank line. Split on "\n\n" boundaries.
    const events = buffer.split("\n\n");
    buffer = events.pop() || "";
    for (const evt of events) {
      const line = evt.split("\n").find((l) => l.startsWith("data:"));
      if (!line) continue;
      const payload = line.slice(5).trim();
      if (!payload) continue;
      try {
        yield JSON.parse(payload);
      } catch {
        // Skip malformed events; the server keeps the stream open.
      }
    }
  }
}

// streamGETSSE opens a GET Server-Sent Events connection and parses it
// incrementally. Uses fetch streaming (rather than the native EventSource)
// because the JWT must be sent via the Authorization header, which
// EventSource cannot set. Same SSE framing as streamSSE.
async function* streamGETSSE(path: string, signal?: AbortSignal): AsyncGenerator<unknown> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    method: "GET",
    credentials: "include",
    headers: {
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal,
  });
  if (!res.ok || !res.body) {
    const txt = await res.text().catch(() => "");
    throw new ApiError(txt || `stream failed: ${res.status}`, res.status);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    // SSE events are separated by a blank line. Split on "\n\n" boundaries.
    const events = buffer.split("\n\n");
    buffer = events.pop() || "";
    for (const evt of events) {
      const line = evt.split("\n").find((l) => l.startsWith("data:"));
      if (!line) continue;
      const payload = line.slice(5).trim();
      if (!payload) continue;
      try {
        yield JSON.parse(payload);
      } catch {
        // Skip malformed events; the server keeps the stream open.
      }
    }
  }
}

// streamDocEvents receives real-time document status updates from
// /documents/events. Used by the document list page to update statuses live.
async function* streamDocEvents(signal?: AbortSignal): AsyncGenerator<DocEvent> {
  yield* streamGETSSE("/documents/events", signal) as AsyncGenerator<DocEvent>;
}
