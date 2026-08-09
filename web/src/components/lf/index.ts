// web/src/components/lf/index.ts
//
// Direction A primitives. Import from `@/components/lf` (or the
// relative path equivalent) instead of from individual files —
// keeps consumer imports tidy and lets us refactor internal file
// layout without breaking every screen.

export { LFLogo } from './LFLogo'
export type { LFLogoProps } from './LFLogo'

export { LFButton } from './LFButton'
export type { LFButtonProps, LFButtonVariant, LFButtonSize } from './LFButton'

export { LFSurface } from './LFSurface'
export type { LFSurfaceProps, LFSurfaceAccent } from './LFSurface'

export { LFAvatar } from './LFAvatar'
export type { LFAvatarProps } from './LFAvatar'

export { LFEpistemic } from './LFEpistemic'
export type { LFEpistemicProps, LFEpistemicKind } from './LFEpistemic'

export { LFTabs } from './LFTabs'
export type { LFTabsProps } from './LFTabs'

export { LFTrustChip } from './LFTrustChip'
export type { LFTrustChipProps } from './LFTrustChip'

export { LFLoomChip } from './LFLoomChip'
export type { LFLoomChipProps } from './LFLoomChip'

export { LFLoomCard } from './LFLoomCard'
export type { LFLoomCardProps } from './LFLoomCard'

export { LFRelatedCard } from './LFRelatedCard'
export type { LFRelatedCardProps, RelatedPost } from './LFRelatedCard'

export { LFEmbeddedArticle } from './LFEmbeddedArticle'
export type { LFEmbeddedArticleProps } from './LFEmbeddedArticle'

export { LFEmbeddedYouTube } from './LFEmbeddedYouTube'
export type { LFEmbeddedYouTubeProps } from './LFEmbeddedYouTube'

export { LFFlair } from './LFFlair'

export { LFRotatedHighlight } from './LFRotatedHighlight'
export type { LFRotatedHighlightProps } from './LFRotatedHighlight'


export { LFSealCheck } from './LFSealCheck'
export type { LFSealCheckProps } from './LFSealCheck'

export { LFCitationBadge } from './LFCitationBadge'
export type { LFCitationBadgeProps } from './LFCitationBadge'

export { LFTrustChart } from './LFTrustChart'
export type { LFTrustChartProps } from './LFTrustChart'

export { LFSideNav } from './LFSideNav'

export { LFRightRail } from './LFRightRail'

export { LFPostCard } from './LFPostCard'
export type { LFPostCardProps } from './LFPostCard'

export { LFPostCardSkeleton, LFPostListSkeleton } from './LFPostCardSkeleton'

export { LFFeedHeader } from './LFFeedHeader'
export type { LFFeedHeaderProps } from './LFFeedHeader'

export { LFQuickCompose } from './LFQuickCompose'
export type { LFQuickComposeProps } from './LFQuickCompose'





export { LFCommunityCard } from './LFCommunityCard'
export type { LFCommunityCardProps, LFCommunityCardCommunity } from './LFCommunityCard'

export { LFCommunityHeader } from './LFCommunityHeader'
export type { LFCommunityHeaderProps } from './LFCommunityHeader'

export { LFSearchInput } from './LFSearchInput'
export type { LFSearchInputProps } from './LFSearchInput'

export { LFFilterChips } from './LFFilterChips'
export type { LFFilterChipsProps, LFFilterChipOption } from './LFFilterChips'

export { LFNotificationItem } from './LFNotificationItem'
export type { LFNotificationItemProps, LFNotificationKind } from './LFNotificationItem'

export { LFConversationListItem } from './LFConversationListItem'
export type { LFConversationListItemProps } from './LFConversationListItem'

export { LFStepIndicator } from './LFStepIndicator'
export type { LFStepIndicatorProps } from './LFStepIndicator'

export { LFPickTile } from './LFPickTile'
export type { LFPickTileProps } from './LFPickTile'

export { LFArenaBattleCard } from './LFArenaBattleCard'
export type { LFArenaBattleCardProps, LFArenaStatus } from './LFArenaBattleCard'

export { LFLeaderboardRow } from './LFLeaderboardRow'
export type { LFLeaderboardRowProps } from './LFLeaderboardRow'

export { LFArenaHeader } from './LFArenaHeader'
export type { LFArenaHeaderProps, LFArenaPhase } from './LFArenaHeader'

export { LFArenaSideColumn } from './LFArenaSideColumn'
export type { LFArenaSideColumnProps, LFArenaSide } from './LFArenaSideColumn'

export { LFArenaVoteBar } from './LFArenaVoteBar'
export type { LFArenaVoteBarProps } from './LFArenaVoteBar'


export { LFBottomNav } from './LFBottomNav'

export { LFMobileDrawer } from './LFMobileDrawer'

export { LFLiveSignal } from './LFLiveSignal'

export { LFAgentMark } from './LFAgentMark'

export { LFSourcesCount } from './LFSourcesCount'

export { LFAgentReputationCard } from './LFAgentReputationCard'

export { LFAgentVoteBar } from './LFAgentVoteBar'

// LFInlineVideo removed — it tried to play extracted inline videos
// via hls.js, but most major news sources (Apple, NYT, etc.) serve
// HLS without CORS headers so the player never worked in practice.
// PostDetail uses the simpler "Play on <domain>" cover overlay
// from #115 instead. Removing the export tree-shakes the ~500KB
// hls.js chunk out of the production bundle.

export { LFInput } from './LFInput'
export type { LFInputProps } from './LFInput'

export { LFTextarea } from './LFTextarea'
export type { LFTextareaProps } from './LFTextarea'

export { LFCommentTree } from './LFCommentTree'
export type { LFCommentTreeProps, CommentNodeView } from './LFCommentTree'

export { LFSourcesStrip } from './LFSourcesStrip'
export type { LFSourcesStripProps } from './LFSourcesStrip'

export { LFVerifyStrip } from './LFVerifyStrip'
export type { LFVerifyStripProps } from './LFVerifyStrip'

export { LFPollCard } from './LFPollCard'
export type { LFPollCardProps } from './LFPollCard'
export { LFPersonRow } from './LFPersonRow'
export type { Person } from './LFPersonRow'

export { default as LFProvenancePanel } from './LFProvenancePanel'

export { SportsCrest } from './SportsCrest'

export { LFSportsHero } from './LFSportsHero'
export type { HeroMatch, HeroTake } from './LFSportsHero'
