import { Citation, ReplyStats, StreamReply, TraceStep } from "@/lib/api";

/** Callbacks for each phase of a chat stream. All are optional; unset
 *  phases are silently ignored. */
export interface StreamCallbacks {
  onCitations?: (cits: Citation[]) => void;
  onToken?: (token: string, annotation?: boolean) => void;
  onThinking?: (token: string) => void;
  onDone?: (messageId: string, stats?: ReplyStats, trace?: TraceStep[]) => void;
  onWarning?: (warning: string) => void;
  onTraceStep?: (step: TraceStep) => void;
  onFollowUps?: (questions: string[]) => void;
  /** Subscription streams only: a user message sent from another client
   *  opened a new turn (messageId + text). */
  onUser?: (messageId: string, text: string) => void;
}

/** Result of consuming a chat stream. `error` is non-null when the stream
 *  reported an error or an unexpected exception occurred (not abort).
 *  `aborted` is true when the consumer cancelled via AbortController. */
export interface StreamResult {
  error: string | null;
  aborted: boolean;
}

/** consumeChatStream reads an async generator of StreamReply events and
 *  dispatches to the provided callbacks. Catches AbortError (from
 *  AbortController) and stream errors, returning a result object instead
 *  of throwing. Used by both the chat page and the agent test drawer. */
export async function consumeChatStream(
  stream: AsyncGenerator<StreamReply>,
  cb: StreamCallbacks
): Promise<StreamResult> {
  let error: string | null = null;
  let aborted = false;
  try {
    while (true) {
      const { value: reply, done } = await stream.next();
      if (done) break;
      if (reply.phase === "retrieve" && reply.citations) {
        cb.onCitations?.(reply.citations);
      } else if (reply.phase === "user" && reply.token) {
        cb.onUser?.(reply.message_id || "", reply.token);
      } else if (reply.phase === "generate" && reply.token) {
        cb.onToken?.(reply.token, reply.annotation);
      } else if (reply.phase === "thinking" && reply.token) {
        cb.onThinking?.(reply.token);
      } else if (reply.phase === "done") {
        cb.onDone?.(reply.message_id || "", reply.stats, reply.trace);
      } else if (reply.phase === "follow_ups" && reply.questions?.length) {
        cb.onFollowUps?.(reply.questions);
      } else if (reply.phase === "trace" && reply.trace?.[0]) {
        cb.onTraceStep?.(reply.trace[0]);
      } else if (reply.phase === "error") {
        error = reply.error || "stream error";
        break;
      } else if (reply.phase === "warning" && reply.warning) {
        cb.onWarning?.(reply.warning);
      }
    }
  } catch (e) {
    if ((e as Error).name !== "AbortError") {
      error = (e as Error).message;
    } else {
      aborted = true;
    }
  }
  return { error, aborted };
}
