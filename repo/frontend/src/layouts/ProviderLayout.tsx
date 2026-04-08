import { Outlet, Link, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";

function ProviderLayout() {
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
            FieldServe — Provider
          </span>
          <div className="flex items-center gap-4">
            <Link
              to="/provider"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Dashboard
            </Link>
            <Link
              to="/provider/services"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Services
            </Link>
            <Link
              to="/provider/interests"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Interests
            </Link>
            <Link
              to="/provider/messages"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Messages
            </Link>
            <Link
              to="/provider/documents"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Documents
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

export default ProviderLayout;
