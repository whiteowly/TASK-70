import { Link } from "react-router-dom";

function NotFoundPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50">
      <h1 className="text-6xl font-bold text-gray-300">404</h1>
      <p className="mt-4 text-lg text-gray-600">Page not found</p>
      <Link
        to="/login"
        className="mt-6 text-blue-600 hover:text-blue-800 hover:underline"
      >
        Go to login
      </Link>
    </div>
  );
}

export default NotFoundPage;
