import { Link } from "react-router-dom";

function ProviderDashboardPage() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Provider Dashboard</h1>
      <p className="mt-2 text-gray-600">
        Welcome to the provider portal.
      </p>
      <div className="mt-4 flex gap-4">
        <Link
          to="/provider/services"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Manage Services</span>
        </Link>
        <Link
          to="/provider/documents"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Documents</span>
        </Link>
      </div>
    </div>
  );
}

export default ProviderDashboardPage;
