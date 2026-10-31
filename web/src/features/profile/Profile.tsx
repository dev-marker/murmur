import { useParams } from 'react-router'

export function Profile() {
  const { handle } = useParams()
  return (
    <>
      <h1 className="bar">Profile</h1>
      <p className="p-4 text-mute">The posts of @{handle}.</p>
    </>
  )
}
