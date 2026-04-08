import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { messageApi, ChatMessage } from "../../api/engagement";
import { useAuthStore } from "../../stores/auth";
import { ApiError } from "../../api/client";

function ProviderThreadDetailPage() {
  const { threadId } = useParams<{ threadId: string }>();
  const queryClient = useQueryClient();
  const user = useAuthStore((s) => s.user);
  const [body, setBody] = useState("");
  const [sendError, setSendError] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ["provider-thread", threadId],
    queryFn: () => messageApi.providerThread(threadId!),
    enabled: Boolean(threadId),
  });

  // Mark as read on mount
  useEffect(() => {
    if (threadId) {
      messageApi.providerMarkRead(threadId).catch(() => {});
    }
  }, [threadId]);

  const sendMut = useMutation({
    mutationFn: () => {
      const key = crypto.randomUUID();
      return messageApi.providerSend(threadId!, body.trim(), key);
    },
    onSuccess: () => {
      setBody("");
      setSendError(null);
      queryClient.invalidateQueries({ queryKey: ["provider-thread", threadId] });
      queryClient.invalidateQueries({ queryKey: ["provider-threads"] });
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.status === 403) {
        setSendError("Cannot send — blocked");
      } else {
        setSendError(err instanceof Error ? err.message : "Send failed");
      }
    },
  });

  const messages: ChatMessage[] = data?.messages ?? [];

  function readStatusLabel(msg: ChatMessage): string | null {
    if (msg.sender_id !== user?.id) return null;
    switch (msg.read_status) {
      case "read":
        return "Read";
      case "delivered":
        return "Delivered";
      default:
        return "Sent";
    }
  }

  return (
    <div className="mx-auto flex max-w-2xl flex-col" style={{ minHeight: "70vh" }}>
      <div className="mb-4">
        <Link
          to="/provider/messages"
          className="text-sm text-blue-600 hover:text-blue-800"
        >
          Back to Messages
        </Link>
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      <div className="flex-1 space-y-3 overflow-y-auto rounded-lg bg-white p-4 shadow-sm">
        {isLoading ? (
          <p className="text-gray-500">Loading...</p>
        ) : messages.length === 0 ? (
          <p className="text-gray-500">No messages yet.</p>
        ) : (
          messages.map((msg) => {
            const isMine = msg.sender_id === user?.id;
            const receipt = readStatusLabel(msg);
            return (
              <div
                key={msg.id}
                className={`flex ${isMine ? "justify-end" : "justify-start"}`}
              >
                <div
                  className={`max-w-[75%] rounded-lg px-3 py-2 text-sm ${
                    isMine
                      ? "bg-blue-600 text-white"
                      : "bg-gray-100 text-gray-900"
                  }`}
                >
                  <p className="mb-0.5 text-xs font-medium opacity-75">
                    {isMine ? "You" : msg.sender_id.slice(0, 8)}
                  </p>
                  <p>{msg.body}</p>
                  <div className="mt-1 flex items-center justify-end gap-2 text-[10px] opacity-60">
                    <span>
                      {new Date(msg.created_at).toLocaleTimeString()}
                    </span>
                    {receipt && (
                      <span data-testid={`receipt-${msg.id}`}>{receipt}</span>
                    )}
                  </div>
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* Composer */}
      <div className="mt-4 rounded-lg bg-white p-4 shadow-sm">
        {sendError && (
          <div className="mb-3 rounded-md bg-red-50 p-2 text-sm text-red-700">
            {sendError}
          </div>
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (body.trim()) sendMut.mutate();
          }}
          className="flex gap-3"
        >
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Type a message..."
            rows={2}
            className="flex-1 resize-none rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <button
            type="submit"
            disabled={sendMut.isPending || !body.trim()}
            className="self-end rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Send
          </button>
        </form>
      </div>
    </div>
  );
}

export default ProviderThreadDetailPage;
