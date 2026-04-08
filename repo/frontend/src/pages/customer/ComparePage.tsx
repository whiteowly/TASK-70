import { Link } from "react-router-dom";
import { useQueries } from "@tanstack/react-query";
import { useCompareStore } from "../../stores/compare";
import { catalogApi } from "../../api/catalog";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

function ComparePage() {
  const items = useCompareStore((s) => s.items);
  const remove = useCompareStore((s) => s.remove);
  const clear = useCompareStore((s) => s.clear);

  // Fetch full detail for each compared service using useQueries (hooks-safe)
  const detailQueries = useQueries({
    queries: items.map((svc) => ({
      queryKey: ["catalog-service-detail", svc.id],
      queryFn: () => catalogApi.getService(svc.id),
      enabled: Boolean(svc.id),
    })),
  });

  if (items.length === 0) {
    return (
      <div>
        <h1 className="mb-6 text-2xl font-bold text-gray-900">
          Compare Services
        </h1>
        <div className="rounded-lg bg-white p-8 text-center shadow-sm">
          <p className="text-gray-500">
            Add services to compare from the catalog.
          </p>
          <Link
            to="/customer/catalog"
            className="mt-3 inline-block text-sm text-blue-600 hover:text-blue-800"
          >
            Browse services
          </Link>
        </div>
      </div>
    );
  }

  function availabilitySummary(idx: number): string {
    const detail = detailQueries[idx]?.data?.service;
    if (!detail || !detail.availability || detail.availability.length === 0)
      return "Not specified";
    const days = [
      ...new Set(detail.availability.map((w) => w.day_of_week)),
    ].sort();
    return days
      .map((d) => {
        const windows = detail.availability.filter(
          (w) => w.day_of_week === d,
        );
        const times = windows
          .map(
            (w) =>
              `${w.start_time.slice(0, 5)}-${w.end_time.slice(0, 5)}`,
          )
          .join(", ");
        return `${DAY_NAMES[d]}: ${times}`;
      })
      .join("; ");
  }

  function serviceAreaDisplay(idx: number): string {
    const miles = items[idx]?.provider?.service_area_miles;
    if (miles != null) return `${miles} miles`;
    const detail = detailQueries[idx]?.data?.service;
    if (detail?.provider?.service_area_miles != null)
      return `${detail.provider.service_area_miles} miles`;
    if (detailQueries[idx]?.isLoading) return "Loading...";
    return "Not specified";
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Compare Services</h1>
        <div className="flex items-center gap-3">
          <Link
            to="/customer/catalog"
            className="text-sm text-blue-600 hover:text-blue-800"
          >
            Back to Catalog
          </Link>
          <button
            onClick={clear}
            className="rounded-md bg-red-50 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-100"
          >
            Clear All
          </button>
        </div>
      </div>

      <div className="overflow-x-auto rounded-lg bg-white shadow-sm">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                Field
              </th>
              {items.map((svc) => (
                <th
                  key={svc.id}
                  className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500"
                >
                  {svc.title}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200">
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Provider
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3 text-sm text-gray-900"
                >
                  {svc.provider.business_name}
                </td>
              ))}
            </tr>
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Price
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3 text-sm font-bold text-gray-900"
                >
                  {formatPrice(svc.price_cents)}
                </td>
              ))}
            </tr>
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Rating
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3 text-sm text-yellow-600"
                >
                  {parseFloat(svc.rating_avg).toFixed(2)} / 5.00
                </td>
              ))}
            </tr>
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Service Area
              </td>
              {items.map((svc, idx) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3 text-sm text-gray-900"
                >
                  {serviceAreaDisplay(idx)}
                </td>
              ))}
            </tr>
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Category
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3 text-sm text-gray-900"
                >
                  {svc.category?.name ?? "Uncategorized"}
                </td>
              ))}
            </tr>
            <tr>
              <td className="px-6 py-3 text-sm font-medium text-gray-700">
                Availability
              </td>
              {items.map((svc, idx) => (
                <td
                  key={svc.id}
                  className="px-6 py-3 text-sm text-gray-900"
                  style={{ maxWidth: "200px" }}
                >
                  {detailQueries[idx]?.isLoading
                    ? "Loading..."
                    : availabilitySummary(idx)}
                </td>
              ))}
            </tr>
            <tr>
              <td className="px-6 py-3 text-sm font-medium text-gray-700">
                Tags
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="px-6 py-3 text-sm text-gray-900"
                >
                  {svc.tags.length > 0
                    ? svc.tags.map((t) => t.name).join(", ")
                    : "None"}
                </td>
              ))}
            </tr>
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Popularity
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3 text-sm text-gray-900"
                >
                  {svc.popularity_score}
                </td>
              ))}
            </tr>
            <tr>
              <td className="whitespace-nowrap px-6 py-3 text-sm font-medium text-gray-700">
                Actions
              </td>
              {items.map((svc) => (
                <td
                  key={svc.id}
                  className="whitespace-nowrap px-6 py-3"
                >
                  <div className="flex items-center gap-2">
                    <Link
                      to={`/customer/catalog/${svc.id}`}
                      className="text-sm text-blue-600 hover:text-blue-800"
                    >
                      View
                    </Link>
                    <button
                      onClick={() => remove(svc.id)}
                      className="text-sm text-red-600 hover:text-red-800"
                    >
                      Remove
                    </button>
                  </div>
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default ComparePage;
