import { Link } from "react-router-dom";

function CustomerDashboardPage() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Customer Dashboard</h1>
      <p className="mt-2 text-gray-600">
        Welcome to the customer portal.
      </p>
      <div className="mt-4">
        <Link
          to="/customer/catalog"
          className="rounded-lg bg-white px-5 py-4 shadow-sm transition hover:shadow-md"
        >
          <span className="text-sm font-medium text-blue-600">Browse Services</span>
        </Link>
      </div>
    </div>
  );
}

export default CustomerDashboardPage;
