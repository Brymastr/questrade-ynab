import { useEffect, useState } from 'react'
import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { api } from './api/client'
import Connect from './pages/Connect'
import Dashboard from './pages/Dashboard'
import Landing from './pages/Landing'
import Mappings from './pages/Mappings'
import type { User } from './types'

export default function App() {
  const [user, setUser] = useState<User | null | undefined>(undefined)

  useEffect(() => {
    api.me().then(setUser).catch(() => setUser(null))
  }, [])

  if (user === undefined) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-blue-600 border-t-transparent" />
      </div>
    )
  }

  return (
    <HashRouter>
      <Routes>
        <Route
          path="/"
          element={
            !user ? (
              <Landing onConnected={setUser} />
            ) : !user.ynab_connected ? (
              <Navigate to="/connect" replace />
            ) : (
              <Navigate to="/dashboard" replace />
            )
          }
        />
        <Route
          path="/connect"
          element={
            !user ? (
              <Navigate to="/" replace />
            ) : (
              <Connect user={user} onConnected={setUser} />
            )
          }
        />
        <Route
          path="/mappings"
          element={
            !user || !user.ynab_connected ? (
              <Navigate to="/" replace />
            ) : (
              <Mappings onSaved={() => (window.location.hash = '/dashboard')} />
            )
          }
        />
        <Route
          path="/dashboard"
          element={
            !user ? (
              <Navigate to="/" replace />
            ) : (
              <Dashboard user={user} onLogout={() => setUser(null)} />
            )
          }
        />
      </Routes>
    </HashRouter>
  )
}
