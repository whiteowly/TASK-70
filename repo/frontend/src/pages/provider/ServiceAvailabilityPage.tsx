import { useState, useEffect } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { providerApi } from "../../api/catalog";
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

interface WindowRow {
  day_of_week: number;
  start_time: string;
  end_time: string;
}

function ServiceAvailabilityPage() {
  const { id } = useParams<{ id: string }>();
  const [windows, setWindows] = useState<WindowRow[]>([]);
  const [apiError, setApiError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const { data: serviceData, isLoading } = useQuery({
    queryKey: ["provider-service", id],
    queryFn: () => providerApi.getService(id!),
    enabled: Boolean(id),
  });

  useEffect(() => {
    if (serviceData?.service?.availability) {
      setWindows(
        serviceData.service.availability.map((w) => ({
          day_of_week: w.day_of_week,
          start_time: w.start_time,
          end_time: w.end_time,
        })),
      );
    }
  }, [serviceData]);

  const saveMutation = useMutation({
    mutationFn: () => providerApi.setAvailability(id!, windows),
    onSuccess: () => {
      setApiError(null);
      setSuccessMsg("Availability saved.");
      setTimeout(() => setSuccessMsg(null), 3000);
    },
    onError: (err: Error) => {
      if (err instanceof ApiError) {
        setApiError(err.message);
      } else {
        setApiError(err.message);
      }
    },
  });

  function addWindow() {
    setWindows([...windows, { day_of_week: 1, start_time: "09:00", end_time: "17:00" }]);
  }

  function removeWindow(index: number) {
    setWindows(windows.filter((_, i) => i !== index));
  }

  function updateWindow(index: number, field: keyof WindowRow, value: string | number) {
    setWindows(
      windows.map((w, i) =>
        i === index ? { ...w, [field]: value } : w,
      ),
    );
  }

  function handleSave() {
    setApiError(null);
    saveMutation.mutate();
  }

  const serviceTitle = serviceData?.service?.title ?? "...";

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-6">
        <Link
          to="/provider/services"
          className="text-sm text-blue-600 hover:text-blue-800"
        >
          Back to Services
        </Link>
      </div>

      <h1 className="mb-6 text-2xl font-bold text-gray-900">
        Availability &mdash; {serviceTitle}
      </h1>

      {apiError && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {apiError}
        </div>
      )}

      {successMsg && (
        <div className="mb-4 rounded-md bg-green-50 p-3 text-sm text-green-700">
          {successMsg}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : (
        <div className="rounded-lg bg-white p-6 shadow-sm">
          {windows.length === 0 ? (
            <p className="mb-4 text-sm text-gray-500">
              No availability windows. Add one below.
            </p>
          ) : (
            <div className="mb-4 space-y-3">
              {windows.map((w, i) => (
                <div
                  key={i}
                  className="flex flex-wrap items-center gap-3 rounded-md border border-gray-200 p-3"
                >
                  <select
                    value={w.day_of_week}
                    onChange={(e) => updateWindow(i, "day_of_week", Number(e.target.value))}
                    className="rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  >
                    {DAY_NAMES.map((name, idx) => (
                      <option key={idx} value={idx}>
                        {name}
                      </option>
                    ))}
                  </select>
                  <input
                    type="time"
                    value={w.start_time}
                    onChange={(e) => updateWindow(i, "start_time", e.target.value)}
                    className="rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  />
                  <span className="text-sm text-gray-400">to</span>
                  <input
                    type="time"
                    value={w.end_time}
                    onChange={(e) => updateWindow(i, "end_time", e.target.value)}
                    className="rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  />
                  <button
                    type="button"
                    onClick={() => removeWindow(i)}
                    className="text-sm font-medium text-red-600 hover:text-red-800"
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}

          <div className="flex gap-3">
            <button
              type="button"
              onClick={addWindow}
              className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
            >
              Add Window
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={saveMutation.isPending}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {saveMutation.isPending ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export default ServiceAvailabilityPage;
