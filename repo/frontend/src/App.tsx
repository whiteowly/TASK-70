import { useEffect } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import LoginPage from "./pages/LoginPage";
import NotFoundPage from "./pages/NotFoundPage";
import CustomerLayout from "./layouts/CustomerLayout";
import ProviderLayout from "./layouts/ProviderLayout";
import AdminLayout from "./layouts/AdminLayout";
import RouteGuard from "./components/RouteGuard";
import CustomerDashboardPage from "./pages/customer/DashboardPage";
import ProviderDashboardPage from "./pages/provider/DashboardPage";
import AdminDashboardPage from "./pages/admin/DashboardPage";
import CategoriesPage from "./pages/admin/CategoriesPage";
import TagsPage from "./pages/admin/TagsPage";
import HotKeywordsPage from "./pages/admin/HotKeywordsPage";
import AutocompletePage from "./pages/admin/AutocompletePage";
import ServicesPage from "./pages/provider/ServicesPage";
import ServiceFormPage from "./pages/provider/ServiceFormPage";
import ServiceAvailabilityPage from "./pages/provider/ServiceAvailabilityPage";
import CatalogPage from "./pages/customer/CatalogPage";
import ServiceDetailPage from "./pages/customer/ServiceDetailPage";
import FavoritesPage from "./pages/customer/FavoritesPage";
import ComparePage from "./pages/customer/ComparePage";
import CustomerInterestsPage from "./pages/customer/InterestsPage";
import CustomerInterestDetailPage from "./pages/customer/InterestDetailPage";
import CustomerThreadsPage from "./pages/customer/ThreadsPage";
import CustomerThreadDetailPage from "./pages/customer/ThreadDetailPage";
import ProviderInterestsPage from "./pages/provider/InterestsPage";
import ProviderThreadsPage from "./pages/provider/ThreadsPage";
import ProviderThreadDetailPage from "./pages/provider/ThreadDetailPage";
import DocumentsPage from "./pages/provider/DocumentsPage";
import AnalyticsPage from "./pages/admin/AnalyticsPage";
import ExportsPage from "./pages/admin/ExportsPage";
import AlertRulesPage from "./pages/admin/AlertRulesPage";
import AlertCenterPage from "./pages/admin/AlertCenterPage";
import WorkOrdersPage from "./pages/admin/WorkOrdersPage";
import WorkOrderDetailPage from "./pages/admin/WorkOrderDetailPage";
import { useAuthStore, primaryRole, roleHomePath } from "./stores/auth";

function RootRedirect() {
  const user = useAuthStore((s) => s.user);
  const path = roleHomePath(primaryRole(user));
  return <Navigate to={path} replace />;
}

function App() {
  const loading = useAuthStore((s) => s.loading);
  const bootstrap = useAuthStore((s) => s.bootstrap);

  useEffect(() => {
    bootstrap();
  }, [bootstrap]);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-gray-500">Loading...</div>
      </div>
    );
  }

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route
        path="/customer/*"
        element={
          <RouteGuard role="customer">
            <CustomerLayout />
          </RouteGuard>
        }
      >
        <Route index element={<CustomerDashboardPage />} />
        <Route path="catalog" element={<CatalogPage />} />
        <Route path="catalog/:id" element={<ServiceDetailPage />} />
        <Route path="favorites" element={<FavoritesPage />} />
        <Route path="compare" element={<ComparePage />} />
        <Route path="interests" element={<CustomerInterestsPage />} />
        <Route path="interests/:id" element={<CustomerInterestDetailPage />} />
        <Route path="messages" element={<CustomerThreadsPage />} />
        <Route path="messages/:threadId" element={<CustomerThreadDetailPage />} />
      </Route>

      <Route
        path="/provider/*"
        element={
          <RouteGuard role="provider">
            <ProviderLayout />
          </RouteGuard>
        }
      >
        <Route index element={<ProviderDashboardPage />} />
        <Route path="services" element={<ServicesPage />} />
        <Route path="services/new" element={<ServiceFormPage />} />
        <Route path="services/:id/edit" element={<ServiceFormPage />} />
        <Route path="services/:id/availability" element={<ServiceAvailabilityPage />} />
        <Route path="interests" element={<ProviderInterestsPage />} />
        <Route path="messages" element={<ProviderThreadsPage />} />
        <Route path="messages/:threadId" element={<ProviderThreadDetailPage />} />
        <Route path="documents" element={<DocumentsPage />} />
      </Route>

      <Route
        path="/admin/*"
        element={
          <RouteGuard role="administrator">
            <AdminLayout />
          </RouteGuard>
        }
      >
        <Route index element={<AdminDashboardPage />} />
        <Route path="categories" element={<CategoriesPage />} />
        <Route path="tags" element={<TagsPage />} />
        <Route path="hot-keywords" element={<HotKeywordsPage />} />
        <Route path="autocomplete" element={<AutocompletePage />} />
        <Route path="analytics" element={<AnalyticsPage />} />
        <Route path="exports" element={<ExportsPage />} />
        <Route path="alert-rules" element={<AlertRulesPage />} />
        <Route path="alerts" element={<AlertCenterPage />} />
        <Route path="work-orders" element={<WorkOrdersPage />} />
        <Route path="work-orders/:id" element={<WorkOrderDetailPage />} />
      </Route>

      <Route path="/" element={<RootRedirect />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}

export default App;
