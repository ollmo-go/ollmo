// Shared type definitions for the ollmo web client.
// All interfaces are derived from the backend API contracts.

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
  // Page anchors map parsed-markdown byte ranges to source pages (MinerU
  // only). JSON: [{"start":0,"end":120,"page":0},...]
  page_anchors?: string;
  // Byte offset of the requested chunk inside `content` (-1 when absent or
  // not located). Used by the viewer to highlight the cited passage.
  anchor_offset?: number;
  anchor_pages?: string;
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
  note?: string;
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

// One model row bound to a provider card. max_tokens/prices are
// chat-specific; embedding/rerank rows report them 0/omitted.
export interface ProviderModelRef {
  id: string;
  name: string;
  model: string;
  context_length?: number;
  max_tokens?: number;
  input_price?: number;
  output_price?: number;
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

// BillMine is the current user's own usage summary + recent rows, served by
// GET /bills/mine for the chat profile dialog.
export interface BillMine {
  overview: BillOverview;
  records: BillRecord[];
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

// Agent execution trace
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
  user_name?: string;
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
