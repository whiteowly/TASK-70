import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { analyticsApi } from "../../api/operations";

function AnalyticsPage() {
  const [growthFrom, setGrowthFrom] = useState("");
  const [growthTo, setGrowthTo] = useState("");
  const [convFrom, setConvFrom] = useState("");
  const [convTo, setConvTo] = useState("");

  const growthQuery = useQuery({
    queryKey: ["analytics-growth", growthFrom, growthTo],
    queryFn: () =>
      analyticsApi.userGrowth(growthFrom || undefined, growthTo || undefined),
  });

  const convQuery = useQuery({
    queryKey: ["analytics-conversion", convFrom, convTo],
    queryFn: () =>
      analyticsApi.conversion(convFrom || undefined, convTo || undefined),
  });

  const utilQuery = useQuery({
    queryKey: ["analytics-utilization"],
    queryFn: () => analyticsApi.providerUtilization(),
  });

  const growthMetrics = growthQuery.data?.metrics ?? [];
  const convMetrics = convQuery.data?.metrics ?? [];
  const providers = utilQuery.data?.providers ?? [];

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Analytics</h1>
      <p className="mt-1 text-gray-600">Platform analytics and metrics.</p>

      {/* User Growth */}
      <section className="mt-8">
        <div className="rounded-lg bg-white p-6 shadow">
          <h2 className="text-lg font-semibold text-gray-900">User Growth</h2>
          <div className="mt-3 flex items-end gap-4">
            <div>
              <label className="block text-xs text-gray-500">From</label>
              <input
                type="date"
                value={growthFrom}
                onChange={(e) => setGrowthFrom(e.target.value)}
                className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500">To</label>
              <input
                type="date"
                value={growthTo}
                onChange={(e) => setGrowthTo(e.target.value)}
                className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
              />
            </div>
          </div>
          <div className="mt-4">
            {growthQuery.isLoading ? (
              <p className="text-sm text-gray-500">Loading...</p>
            ) : growthMetrics.length === 0 ? (
              <p className="text-sm text-gray-500">No data for this period.</p>
            ) : (
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                      Date
                    </th>
                    <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                      Role
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Count
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200">
                  {growthMetrics.map((m, i) => (
                    <tr key={i}>
                      <td className="px-4 py-2 text-sm text-gray-900">
                        {m.date}
                      </td>
                      <td className="px-4 py-2 text-sm text-gray-600">
                        {m.label}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {m.value}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </section>

      {/* Search-to-Interest Conversion */}
      <section className="mt-8">
        <div className="rounded-lg bg-white p-6 shadow">
          <h2 className="text-lg font-semibold text-gray-900">
            Search-to-Interest Conversion
          </h2>
          <div className="mt-3 flex items-end gap-4">
            <div>
              <label className="block text-xs text-gray-500">From</label>
              <input
                type="date"
                value={convFrom}
                onChange={(e) => setConvFrom(e.target.value)}
                className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500">To</label>
              <input
                type="date"
                value={convTo}
                onChange={(e) => setConvTo(e.target.value)}
                className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
              />
            </div>
          </div>
          <div className="mt-4">
            {convQuery.isLoading ? (
              <p className="text-sm text-gray-500">Loading...</p>
            ) : convMetrics.length === 0 ? (
              <p className="text-sm text-gray-500">No data for this period.</p>
            ) : (
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                      Date
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Searches
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Interests
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Conversion Rate
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200">
                  {convMetrics.map((m, i) => (
                    <tr key={i}>
                      <td className="px-4 py-2 text-sm text-gray-900">
                        {m.date}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {m.searches}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {m.interests}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {(m.rate * 100).toFixed(1)}%
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </section>

      {/* Provider Utilization */}
      <section className="mt-8">
        <div className="rounded-lg bg-white p-6 shadow">
          <h2 className="text-lg font-semibold text-gray-900">
            Provider Utilization
          </h2>
          <div className="mt-4">
            {utilQuery.isLoading ? (
              <p className="text-sm text-gray-500">Loading...</p>
            ) : providers.length === 0 ? (
              <p className="text-sm text-gray-500">No provider data.</p>
            ) : (
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                      Provider
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Active Services
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Total Interests
                    </th>
                    <th className="px-4 py-2 text-right text-xs font-medium uppercase text-gray-500">
                      Messages Sent
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200">
                  {providers.map((p) => (
                    <tr key={p.id}>
                      <td className="px-4 py-2 text-sm text-gray-900">
                        {p.business_name}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {p.active_services}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {p.total_interests}
                      </td>
                      <td className="px-4 py-2 text-right text-sm text-gray-900">
                        {p.messages_sent}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}

export default AnalyticsPage;
