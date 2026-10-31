import { createBrowserRouter } from 'react-router'
import { Layout } from './components/Layout'
import { NotFound } from './components/ui/Notice'
import { Home } from './features/feed/Home'
import { Notifications } from './features/notifications/Notifications'
import { Profile } from './features/profile/Profile'

export const router = createBrowserRouter([
  {
    Component: Layout,
    children: [
      { index: true, Component: Home },
      { path: 'u/:handle', Component: Profile },
      { path: 'notifications', Component: Notifications },
      { path: '*', Component: NotFound },
    ],
  },
])
