import { Outlet, Link, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";
import { useCompareStore } from "../stores/compare";

function CustomerLayout() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const compareCount = useCompareStore((s) => s.items.length);

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="border-b bg-white px-6 py-3">
        <div className="flex items-center justify-between">
          <span className="text-lg font-semibold text-gray-900">
            FieldServe — Customer
          </span>
          <div className="flex items-center gap-4">
            <Link
              to="/customer"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Dashboard
            </Link>
            <Link
              to="/customer/catalog"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Search
            </Link>
            <Link
              to="/customer/favorites"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Favorites
            </Link>
            <Link
              to="/customer/interests"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Interests
            </Link>
            <Link
              to="/customer/messages"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Messages
            </Link>
            <Link
              to="/customer/compare"
              className="text-sm text-gray-600 hover:text-gray-900"
            >
              Compare
              {compareCount > 0 && (
                <span className="ml-1 inline-flex h-5 w-5 items-center justify-center rounded-full bg-blue-600 text-xs font-medium text-white">
                  {compareCount}
                </span>
              )}
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

export default CustomerLayout;
