import { useMutation } from '@tanstack/react-query'
import { request } from '../../lib/api'

type Props = { handle: string; following: boolean }

// The button shows what you asked for while the server works on it,
// and the refetches after it bring the counts.
export function FollowButton({ handle, following }: Props) {
  const follow = useMutation({
    mutationFn: (on: boolean) =>
      request(`/users/${handle}/follow`, {
        method: on ? 'POST' : 'DELETE',
      }),
    onSettled: (_data, _error, _on, _result, { client }) =>
      Promise.all([
        client.invalidateQueries({ queryKey: ['user', handle] }),
        client.invalidateQueries({ queryKey: ['suggestions'] }),
        client.invalidateQueries({ queryKey: ['posts', 'home'] }),
      ]),
  })
  const on = follow.isPending ? follow.variables : following
  return (
    <button
      onClick={() => follow.mutate(!on)}
      className={on ? 'btn-quiet text-mute' : 'btn-quiet border-mute'}
    >
      {on ? 'Following' : 'Follow'}
    </button>
  )
}
