import type { Metadata } from 'next'
import ArenaBattle from '../../../views/ArenaBattle'

export const metadata: Metadata = {
  title: 'Debate — The Arena',
  description: 'Watch two contributors debate head-to-head. Vote on each round and decide who carried the case.',
}

export default function ArenaBattlePage() {
  return <ArenaBattle />
}
