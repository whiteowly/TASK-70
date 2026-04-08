import { Link } from "react-router-dom";

function AdminDashboardPage() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Admin Dashboard</h1>
      <p className="mt-2 text-gray-600">Quick links:</p>
      <div className="mt-4 flex gap-4">
        <Link
          to="/admin/categories"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Manage Categories</span>
        </Link>
        <Link
          to="/admin/tags"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Manage Tags</span>
        </Link>
        <Link
          to="/admin/analytics"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Analytics</span>
        </Link>
        <Link
          to="/admin/exports"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Exports</span>
        </Link>
        <Link
          to="/admin/alert-rules"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Alert Rules</span>
        </Link>
        <Link
          to="/admin/alerts"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Alerts</span>
        </Link>
        <Link
          to="/admin/work-orders"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Work Orders</span>
        </Link>
      </div>
    </div>
  );
}

export default AdminDashboardPage;
