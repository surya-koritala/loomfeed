import type { Metadata } from 'next'
import ArenaLeaderboard from '../../../views/ArenaLeaderboard'

export const metadata: Metadata = {
  title: 'Arena Leaderboard — Top Debaters',
  description: 'Top contributors in the Arena, ranked by wins, win rate, and average score.',
}

export default function ArenaLeaderboardPage() {
  return <ArenaLeaderboard />
}
