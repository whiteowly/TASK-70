import { useParams, Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { interestApi, blockApi } from "../../api/engagement";

const STATUS_COLORS: Record<string, string> = {
  submitted: "bg-blue-100 text-blue-800",
  accepted: "bg-green-100 text-green-800",
  declined: "bg-red-100 text-red-800",
  withdrawn: "bg-gray-100 text-gray-800",
};

function InterestDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ["customer-interest", id],
    queryFn: () => interestApi.customerGet(id!),
    enabled: Boolean(id),
  });

  const withdrawMut = useMutation({
    mutationFn: () => interestApi.customerWithdraw(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["customer-interest", id] });
      queryClient.invalidateQueries({ queryKey: ["customer-interests"] });
    },
    onError: (err: unknown) => {
      setActionError(err instanceof Error ? err.message : "Action failed");
    },
  });

  const blockMut = useMutation({
    mutationFn: () => blockApi.customerBlock(data!.interest.provider_id),
    onSuccess: () => setActionError(null),
    onError: (err: unknown) => {
      setActionError(err instanceof Error ? err.message : "Block failed");
    },
  });

  const unblockMut = useMutation({
    mutationFn: () => blockApi.customerUnblock(data!.interest.provider_id),
    onSuccess: () => setActionError(null),
    onError: (err: unknown) => {
      setActionError(err instanceof Error ? err.message : "Unblock failed");
    },
  });

  const interest = data?.interest;
  const events = data?.events ?? [];

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-6">
        <Link
          to="/customer/interests"
          className="text-sm text-blue-600 hover:text-blue-800"
        >
          Back to Interests
        </Link>
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}
      {actionError && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {actionError}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : !interest ? (
        <p className="text-gray-500">Interest not found.</p>
      ) : (
        <div className="space-y-6">
          <div className="rounded-lg bg-white p-6 shadow-sm">
            <div className="mb-4 flex items-start justify-between">
              <div>
                <h1 className="text-xl font-bold text-gray-900">
                  Interest Detail
                </h1>
                <p className="mt-1 text-sm text-gray-500">
                  ID: {interest.id}
                </p>
              </div>
              <span
                data-testid="interest-status"
                className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_COLORS[interest.status] ?? "bg-gray-100 text-gray-800"}`}
              >
                {interest.status}
              </span>
            </div>

            <div className="mb-4 grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Service
                </span>
                <span className="text-gray-900">{interest.service_id}</span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Provider
                </span>
                <span className="text-gray-900">{interest.provider_id}</span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Created
                </span>
                <span className="text-gray-900">
                  {new Date(interest.created_at).toLocaleString()}
                </span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Updated
                </span>
                <span className="text-gray-900">
                  {new Date(interest.updated_at).toLocaleString()}
                </span>
              </div>
            </div>

            <div className="flex items-center gap-3">
              {(interest.status === "submitted" ||
                interest.status === "accepted") && (
                <button
                  onClick={() => withdrawMut.mutate()}
                  disabled={withdrawMut.isPending}
                  className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 disabled:opacity-50"
                >
                  Withdraw
                </button>
              )}

              {(interest.status === "submitted" ||
                interest.status === "accepted") && (
                <Link
                  to={`/customer/messages/${interest.id}`}
                  className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
                >
                  Send Message
                </Link>
              )}

              <button
                onClick={() => blockMut.mutate()}
                disabled={blockMut.isPending}
                className="rounded-md bg-red-50 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-100 disabled:opacity-50"
              >
                Block Provider
              </button>

              <button
                onClick={() => unblockMut.mutate()}
                disabled={unblockMut.isPending}
                className="rounded-md bg-gray-50 px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 disabled:opacity-50"
              >
                Unblock Provider
              </button>
            </div>
          </div>

          {/* Timeline */}
          <div className="rounded-lg bg-white p-6 shadow-sm">
            <h2 className="mb-4 text-lg font-semibold text-gray-900">
              Status Timeline
            </h2>
            {events.length === 0 ? (
              <p className="text-sm text-gray-500">No events yet.</p>
            ) : (
              <ol className="space-y-3">
                {events.map((event) => (
                  <li
                    key={event.id}
                    className="flex items-start gap-3 border-l-2 border-gray-200 pl-4"
                  >
                    <div>
                      <p className="text-sm text-gray-900">
                        {event.old_status ? (
                          <>
                            <span className="font-medium">
                              {event.old_status}
                            </span>{" "}
                            &rarr;{" "}
                          </>
                        ) : null}
                        <span className="font-medium">{event.new_status}</span>
                      </p>
                      <p className="text-xs text-gray-500">
                        {new Date(event.created_at).toLocaleString()}
                      </p>
                    </div>
                  </li>
                ))}
              </ol>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

export default InterestDetailPage;
