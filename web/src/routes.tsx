import { createBrowserRouter, redirect } from 'react-router'
import { Layout } from './components/Layout'
import { NotFound } from './components/ui/Notice'
import { Welcome } from './features/auth/Welcome'
import { Home } from './features/feed/Home'
import { Notifications } from './features/notifications/Notifications'
import { Profile } from './features/profile/Profile'
import { getSession } from './lib/api'

// Without a token everything is the welcome page, and with one the
// welcome page is home. Loaders decide before anything renders.
export const router = createBrowserRouter([
  {
    path: '/welcome',
    loader: () => (getSession() ? redirect('/') : null),
    Component: Welcome,
  },
  {
    loader: () => (getSession() ? null : redirect('/welcome')),
    Component: Layout,
    children: [
      { index: true, Component: Home },
      { path: 'u/:handle', Component: Profile },
      { path: 'notifications', Component: Notifications },
      { path: '*', Component: NotFound },
    ],
  },
])
