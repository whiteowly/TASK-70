import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { interestApi, Interest } from "../../api/engagement";

const STATUS_COLORS: Record<string, string> = {
  submitted: "bg-blue-100 text-blue-800",
  accepted: "bg-green-100 text-green-800",
  declined: "bg-red-100 text-red-800",
  withdrawn: "bg-gray-100 text-gray-800",
};

function InterestsPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ["customer-interests"],
    queryFn: interestApi.customerList,
  });

  const withdrawMut = useMutation({
    mutationFn: (id: string) => interestApi.customerWithdraw(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["customer-interests"] }),
  });

  const interests: Interest[] = data?.interests ?? [];

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-6 text-2xl font-bold text-gray-900">My Interests</h1>

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : interests.length === 0 ? (
        <p className="text-gray-500">No interests yet.</p>
      ) : (
        <div className="space-y-3">
          {interests.map((interest) => (
            <div
              key={interest.id}
              className="flex items-center justify-between rounded-lg bg-white p-4 shadow-sm"
            >
              <div>
                <Link
                  to={`/customer/interests/${interest.id}`}
                  className="text-sm font-medium text-blue-600 hover:text-blue-800"
                >
                  Interest #{interest.id.slice(0, 8)}
                </Link>
                <p className="mt-1 text-xs text-gray-500">
                  Service: {interest.service_id.slice(0, 8)} &middot;{" "}
                  {new Date(interest.created_at).toLocaleDateString()}
                </p>
              </div>
              <div className="flex items-center gap-3">
                <span
                  data-testid={`status-badge-${interest.id}`}
                  className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_COLORS[interest.status] ?? "bg-gray-100 text-gray-800"}`}
                >
                  {interest.status}
                </span>
                {(interest.status === "submitted" ||
                  interest.status === "accepted") && (
                  <button
                    onClick={() => withdrawMut.mutate(interest.id)}
                    disabled={withdrawMut.isPending}
                    className="rounded-md bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-200 disabled:opacity-50"
                  >
                    Withdraw
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default InterestsPage;
