import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { messageApi, Thread } from "../../api/engagement";

function ThreadsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["customer-threads"],
    queryFn: messageApi.customerThreads,
  });

  const threads: Thread[] = data?.threads ?? [];

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-6 text-2xl font-bold text-gray-900">Messages</h1>

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : threads.length === 0 ? (
        <p className="text-gray-500">No conversations yet.</p>
      ) : (
        <div className="space-y-2">
          {threads.map((thread) => (
            <Link
              key={thread.thread_id}
              to={`/customer/messages/${thread.thread_id}`}
              className="flex items-center justify-between rounded-lg bg-white p-4 shadow-sm hover:bg-gray-50"
            >
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-gray-900">
                  {thread.other_name}
                </p>
                <p className="mt-0.5 truncate text-xs text-gray-500">
                  {thread.last_message}
                </p>
              </div>
              <div className="ml-4 flex flex-shrink-0 items-center gap-3">
                <span className="text-xs text-gray-400">
                  {new Date(thread.last_at).toLocaleDateString()}
                </span>
                {thread.unread_count > 0 && (
                  <span
                    data-testid={`unread-${thread.thread_id}`}
                    className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-blue-600 px-1.5 text-xs font-medium text-white"
                  >
                    {thread.unread_count}
                  </span>
                )}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

export default ThreadsPage;
