import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { request, setSession, type Me } from '../../lib/api'

type Account = Me & { token: string }

// field hands a form's one field to run. This is onSubmit, not a form
// action: an action resets the form, and a taken handle should stay.
const field =
  (name: string, run: (value: string) => void) =>
  (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    run(String(new FormData(e.currentTarget).get(name)).trim())
  }

export function Welcome() {
  const navigate = useNavigate()
  const [copied, setCopied] = useState(false)
  const signUp = useMutation({
    mutationFn: (handle: string) =>
      request<Account>('/users', { method: 'POST', json: { handle } }),
    onSuccess: ({ token, handle }) => setSession({ token, handle }),
  })
  const signIn = useMutation({
    mutationFn: (token: string) =>
      request<Me>('/me', {
        headers: { Authorization: `Bearer ${token}` },
      }),
    onSuccess: ({ handle }, token) => {
      setSession({ token, handle })
      navigate('/')
    },
  })
  const account = signUp.data

  return (
    <main
      className="mx-auto grid min-h-dvh max-w-4xl content-center
        items-center gap-12 px-6 py-16 lg:grid-cols-2 lg:gap-20"
    >
      <header>
        <p className="text-5xl font-semibold tracking-tight lg:text-6xl">
          murmur<span className="text-accent-ink">.</span>
        </p>
        <p className="mt-4 max-w-xs text-lg text-mute">
          Short posts from a few people you follow, and a quiet word
          when someone likes yours.
        </p>
      </header>

      {account ? (
        <section className="space-y-4">
          <h1 className="text-lg font-semibold">
            Welcome, @{account.handle}
          </h1>
          <p className="text-sm text-mute">
            This is your key. There is no password: keep the key
            somewhere safe, because it is the only way back in, and it
            is shown only once.
          </p>
          <code
            className="block rounded-lg border border-line p-3 text-sm
              break-all"
          >
            {account.token}
          </code>
          <div className="flex gap-3">
            <button
              className="btn-quiet"
              onClick={async () => {
                await navigator.clipboard.writeText(account.token)
                setCopied(true)
              }}
            >
              {copied ? 'Copied' : 'Copy key'}
            </button>
            <button className="btn" onClick={() => navigate('/')}>
              Continue
            </button>
          </div>
        </section>
      ) : (
        <div className="space-y-10">
          <form
            onSubmit={field('handle', signUp.mutate)}
            className="space-y-3"
          >
            <h1 className="text-lg font-semibold">Join Murmur</h1>
            <label className="block text-sm">
              <span className="text-mute">Pick a handle</span>
              <input
                name="handle"
                required
                autoCapitalize="none"
                spellCheck={false}
                placeholder="your_name"
                className="field mt-1"
              />
            </label>
            <Problem error={signUp.error} />
            <button className="btn" disabled={signUp.isPending}>
              Create account
            </button>
          </form>

          <form
            onSubmit={field('token', signIn.mutate)}
            className="space-y-3 border-t border-line pt-8 text-sm"
          >
            <h2 className="font-medium">Have a key?</h2>
            <div className="flex gap-2">
              <label className="flex-1">
                <span className="sr-only">Your key</span>
                <input
                  name="token"
                  required
                  type="password"
                  autoComplete="off"
                  placeholder="Paste it here"
                  className="field"
                />
              </label>
              <button className="btn-quiet" disabled={signIn.isPending}>
                Sign in
              </button>
            </div>
            <Problem error={signIn.error} />
          </form>
        </div>
      )}
    </main>
  )
}

// Problem says what the API said, the way it said it.
function Problem({ error }: { error: Error | null }) {
  if (!error) return null
  return (
    <p
      role="alert"
      className="text-sm text-accent-ink first-letter:uppercase"
    >
      {error.message}
    </p>
  )
}
