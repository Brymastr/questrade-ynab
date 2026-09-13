import { useEffect, useState } from 'react'
import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { api } from './api/client'
import Spinner from './components/ui/Spinner'
import ConnectQuestrade from './pages/ConnectQuestrade'
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
      <div className="flex h-screen items-center justify-center bg-bg text-fg-faint">
        <Spinner className="h-6 w-6" />
      </div>
    )
  }

  return (
    <HashRouter>
      <Routes>
        <Route
          path="/"
          element={!user ? <Landing /> : <Navigate to="/dashboard" replace />}
        />
        {/* Questrade token prompt; connecting creates the session. */}
        <Route
          path="/connect"
          element={
            user?.questrade_connected ? (
              <Navigate to="/dashboard" replace />
            ) : (
              <ConnectQuestrade onConnected={setUser} />
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
        {/* Any unknown hash falls back to home instead of a blank screen. */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </HashRouter>
  )
}
