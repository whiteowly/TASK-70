import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { catalogApi, customerApi } from "../../api/catalog";
import { useCompareStore, MAX_COMPARE } from "../../stores/compare";
import { interestApi, blockApi } from "../../api/engagement";
import { ApiError } from "../../api/client";

const DAY_NAMES = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

function ServiceDetailPage() {
  const { id } = useParams<{ id: string }>();

  const { data, isLoading, error } = useQuery({
    queryKey: ["catalog-service", id],
    queryFn: () => catalogApi.getService(id!),
    enabled: Boolean(id),
  });

  const service = data?.service;

  // Favorites
  const [isFavorite, setIsFavorite] = useState(false);
  const { data: favoritesData } = useQuery({
    queryKey: ["customer-favorites"],
    queryFn: customerApi.getFavorites,
  });

  useEffect(() => {
    if (favoritesData?.favorites && id) {
      setIsFavorite(favoritesData.favorites.some((f) => f.service_id === id));
    }
  }, [favoritesData, id]);

  const toggleFavorite = useCallback(async () => {
    if (!id) return;
    const wasFav = isFavorite;
    setIsFavorite(!wasFav);
    try {
      if (wasFav) {
        await customerApi.removeFavorite(id);
      } else {
        await customerApi.addFavorite(id);
      }
    } catch {
      setIsFavorite(wasFav);
    }
  }, [id, isFavorite]);

  // Compare
  const compareAdd = useCompareStore((s) => s.add);
  const compareRemove = useCompareStore((s) => s.remove);
  const compareHas = useCompareStore((s) => s.has);
  const inCompare = id ? compareHas(id) : false;
  const [compareAlert, setCompareAlert] = useState<string | null>(null);

  const toggleCompare = useCallback(() => {
    if (!service) return;
    if (inCompare) {
      compareRemove(service.id);
      setCompareAlert(null);
    } else {
      const added = compareAdd(service);
      if (!added) {
        setCompareAlert(
          `Compare is limited to ${MAX_COMPARE} services. Remove one to add another.`,
        );
        setTimeout(() => setCompareAlert(null), 4000);
      } else {
        setCompareAlert(null);
      }
    }
  }, [service, inCompare, compareAdd, compareRemove]);

  // Interest submission
  const [interestSuccess, setInterestSuccess] = useState(false);
  const [interestError, setInterestError] = useState<string | null>(null);
  const [isBlocked, setIsBlocked] = useState(false);

  const submitInterest = useMutation({
    mutationFn: () => {
      const key = crypto.randomUUID();
      return interestApi.customerSubmit(
        { service_id: id!, provider_id: service!.provider.id },
        key,
      );
    },
    onSuccess: () => {
      setInterestSuccess(true);
      setInterestError(null);
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError) {
        if (err.status === 403) {
          setIsBlocked(true);
          setInterestError("Blocked");
        } else if (err.code === "duplicate_interest") {
          const providerErrors = err.fieldErrors?.provider_id;
          setInterestError(
            providerErrors?.length
              ? providerErrors.join(" ")
              : err.message,
          );
        } else {
          setInterestError(err.message);
        }
      } else {
        setInterestError(
          err instanceof Error ? err.message : "Failed to submit interest",
        );
      }
    },
  });

  const blockProviderMut = useMutation({
    mutationFn: () => blockApi.customerBlock(service!.provider.id),
    onSuccess: () => setIsBlocked(true),
  });

  const unblockProviderMut = useMutation({
    mutationFn: () => blockApi.customerUnblock(service!.provider.id),
    onSuccess: () => setIsBlocked(false),
  });

  function formatPrice(cents: number): string {
    return `$${(cents / 100).toFixed(2)}`;
  }

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-6">
        <Link
          to="/customer/catalog"
          className="text-sm text-blue-600 hover:text-blue-800"
        >
          Back to Catalog
        </Link>
      </div>

      {compareAlert && (
        <div role="alert" className="mb-4 rounded-md bg-amber-50 p-3 text-sm text-amber-800">
          {compareAlert}
        </div>
      )}

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : !service ? (
        <p className="text-gray-500">Service not found.</p>
      ) : (
        <div className="space-y-6">
          <div className="rounded-lg bg-white p-6 shadow-sm">
            <div className="mb-4 flex items-start justify-between">
              <div>
                <h1 className="mb-2 text-2xl font-bold text-gray-900">
                  {service.title}
                </h1>
                <p className="text-sm text-gray-500">
                  by {service.provider.business_name}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={toggleFavorite}
                  className={`rounded-md px-3 py-1.5 text-xs font-medium ${
                    isFavorite
                      ? "bg-red-50 text-red-600 hover:bg-red-100"
                      : "bg-gray-50 text-gray-600 hover:bg-gray-100"
                  }`}
                >
                  {isFavorite ? "Favorited" : "Favorite"}
                </button>
                <button
                  onClick={toggleCompare}
                  className={`rounded-md px-3 py-1.5 text-xs font-medium ${
                    inCompare
                      ? "bg-blue-50 text-blue-600 hover:bg-blue-100"
                      : "bg-gray-50 text-gray-600 hover:bg-gray-100"
                  }`}
                >
                  {inCompare ? "In Compare" : "+ Compare"}
                </button>
              </div>
            </div>

            {service.description && (
              <p className="mb-4 text-gray-700">{service.description}</p>
            )}

            <div className="mb-4 grid grid-cols-2 gap-4">
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Category
                </span>
                <span className="text-sm text-gray-900">
                  {service.category?.name ?? "Uncategorized"}
                </span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Price
                </span>
                <span className="text-lg font-bold text-gray-900">
                  {formatPrice(service.price_cents)}
                </span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Rating
                </span>
                <span className="text-sm text-yellow-600">
                  {service.rating_avg}
                </span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase text-gray-400">
                  Status
                </span>
                <span
                  className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                    service.status === "active"
                      ? "bg-green-100 text-green-800"
                      : "bg-gray-100 text-gray-800"
                  }`}
                >
                  {service.status}
                </span>
              </div>
            </div>

            {service.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {service.tags.map((tag) => (
                  <span
                    key={tag.id}
                    className="inline-block rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-700"
                  >
                    {tag.name}
                  </span>
                ))}
              </div>
            )}
          </div>

          {/* Interest & Block Actions */}
          <div className="rounded-lg bg-white p-6 shadow-sm">
            <h2 className="mb-4 text-lg font-semibold text-gray-900">
              Interested?
            </h2>

            {isBlocked && (
              <p className="mb-3 text-sm font-medium text-red-600">Blocked</p>
            )}

            {interestSuccess && (
              <p className="mb-3 text-sm font-medium text-green-600">
                Interest submitted successfully!
              </p>
            )}

            {interestError && (
              <p className="mb-3 text-sm text-red-600">{interestError}</p>
            )}

            <div className="flex items-center gap-3">
              <button
                onClick={() => submitInterest.mutate()}
                disabled={
                  submitInterest.isPending || interestSuccess || isBlocked
                }
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              >
                Submit Interest
              </button>

              {!isBlocked ? (
                <button
                  onClick={() => blockProviderMut.mutate()}
                  disabled={blockProviderMut.isPending}
                  className="rounded-md bg-red-50 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-100 disabled:opacity-50"
                >
                  Block Provider
                </button>
              ) : (
                <button
                  onClick={() => unblockProviderMut.mutate()}
                  disabled={unblockProviderMut.isPending}
                  className="rounded-md bg-gray-50 px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 disabled:opacity-50"
                >
                  Unblock Provider
                </button>
              )}
            </div>
          </div>

          {service.availability && service.availability.length > 0 && (
            <div className="rounded-lg bg-white p-6 shadow-sm">
              <h2 className="mb-4 text-lg font-semibold text-gray-900">
                Availability
              </h2>
              <div className="overflow-hidden rounded-md border border-gray-200">
                <table className="min-w-full divide-y divide-gray-200">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                        Day
                      </th>
                      <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                        Start
                      </th>
                      <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                        End
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {service.availability.map((w) => (
                      <tr key={w.id}>
                        <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-900">
                          {DAY_NAMES[w.day_of_week] ?? w.day_of_week}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-600">
                          {w.start_time}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-600">
                          {w.end_time}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default ServiceDetailPage;
