import { Outlet, Link, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";

function AdminLayout() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="border-b bg-white px-6 py-3">
        <div className="flex items-center justify-between">
          <span className="text-lg font-semibold text-gray-900">
            FieldServe — Admin
          </span>
          <div className="flex items-center gap-4">
            <Link
              to="/admin"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Dashboard
            </Link>
            <Link
              to="/admin/categories"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Categories
            </Link>
            <Link
              to="/admin/tags"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Tags
            </Link>
            <Link
              to="/admin/hot-keywords"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Hot Keywords
            </Link>
            <Link
              to="/admin/autocomplete"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Autocomplete
            </Link>
            <Link
              to="/admin/analytics"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Analytics
            </Link>
            <Link
              to="/admin/exports"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Exports
            </Link>
            <Link
              to="/admin/alert-rules"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Alert Rules
            </Link>
            <Link
              to="/admin/alerts"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Alerts
            </Link>
            <Link
              to="/admin/work-orders"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Work Orders
            </Link>
            <span className="text-sm text-gray-400">{user?.username}</span>
            <button
              onClick={handleLogout}
              className="text-sm text-red-600 hover:text-red-800"
            >
              Logout
            </button>
          </div>
        </div>
      </nav>
      <main className="p-6">
        <Outlet />
      </main>
    </div>
  );
}

export default AdminLayout;
