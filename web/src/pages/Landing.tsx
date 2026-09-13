import FirstRun from '../components/FirstRun'
import PageShell from '../components/ui/PageShell'

// Logged-out home: the same checklist the dashboard shows on first run, with
// nothing ticked yet. Step one links to the Questrade token prompt.
export default function Landing() {
  return (
    <PageShell variant="centered" maxWidth="max-w-md">
      <p className="mb-8 text-center font-mono text-xs text-fg-faint">questrade → ynab</p>
      <FirstRun user={null} />
    </PageShell>
  )
}
