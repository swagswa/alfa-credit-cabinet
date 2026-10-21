import { Navigate, Route, Routes } from 'react-router';
import { Login } from './pages/Login';
import { ClientCabinet } from './pages/ClientCabinet';
import { BankCabinet } from './pages/BankCabinet';

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/client/*" element={<ClientCabinet />} />
      <Route path="/bank/*" element={<BankCabinet />} />
      <Route path="*" element={<Navigate to="/login" replace />} />
    </Routes>
  );
}
