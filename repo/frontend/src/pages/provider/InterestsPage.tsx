import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { interestApi, blockApi, Interest } from "../../api/engagement";
import { useState } from "react";

const STATUS_COLORS: Record<string, string> = {
  submitted: "bg-blue-100 text-blue-800",
  accepted: "bg-green-100 text-green-800",
  declined: "bg-red-100 text-red-800",
  withdrawn: "bg-gray-100 text-gray-800",
};

function ProviderInterestsPage() {
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ["provider-interests"],
    queryFn: interestApi.providerList,
  });

  const acceptMut = useMutation({
    mutationFn: (id: string) => interestApi.providerAccept(id),
    onSuccess: () => {
      setActionError(null);
      queryClient.invalidateQueries({ queryKey: ["provider-interests"] });
    },
    onError: (err: unknown) => {
      setActionError(err instanceof Error ? err.message : "Action failed");
    },
  });

  const declineMut = useMutation({
    mutationFn: (id: string) => interestApi.providerDecline(id),
    onSuccess: () => {
      setActionError(null);
      queryClient.invalidateQueries({ queryKey: ["provider-interests"] });
    },
    onError: (err: unknown) => {
      setActionError(err instanceof Error ? err.message : "Action failed");
    },
  });

  const blockMut = useMutation({
    mutationFn: (customerId: string) => blockApi.providerBlock(customerId),
    onSuccess: () => setActionError(null),
    onError: (err: unknown) => {
      setActionError(err instanceof Error ? err.message : "Block failed");
    },
  });

  const interests: Interest[] = data?.interests ?? [];

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-6 text-2xl font-bold text-gray-900">
        Incoming Interests
      </h1>

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
      ) : interests.length === 0 ? (
        <p className="text-gray-500">No incoming interests.</p>
      ) : (
        <div className="space-y-3">
          {interests.map((interest) => (
            <div
              key={interest.id}
              className="rounded-lg bg-white p-4 shadow-sm"
            >
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium text-gray-900">
                    Interest #{interest.id.slice(0, 8)}
                  </p>
                  <p className="mt-0.5 text-xs text-gray-500">
                    Customer: {interest.customer_id.slice(0, 8)} &middot;
                    Service: {interest.service_id.slice(0, 8)} &middot;{" "}
                    {new Date(interest.created_at).toLocaleDateString()}
                  </p>
                </div>
                <span
                  data-testid={`status-badge-${interest.id}`}
                  className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_COLORS[interest.status] ?? "bg-gray-100 text-gray-800"}`}
                >
                  {interest.status}
                </span>
              </div>

              {interest.status === "submitted" && (
                <div className="mt-3 flex items-center gap-2">
                  <button
                    onClick={() => acceptMut.mutate(interest.id)}
                    disabled={acceptMut.isPending}
                    className="rounded-md bg-green-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50"
                  >
                    Accept
                  </button>
                  <button
                    onClick={() => declineMut.mutate(interest.id)}
                    disabled={declineMut.isPending}
                    className="rounded-md bg-red-50 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 disabled:opacity-50"
                  >
                    Decline
                  </button>
                  <button
                    onClick={() => blockMut.mutate(interest.customer_id)}
                    disabled={blockMut.isPending}
                    className="rounded-md bg-gray-50 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-100 disabled:opacity-50"
                  >
                    Block Customer
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default ProviderInterestsPage;
