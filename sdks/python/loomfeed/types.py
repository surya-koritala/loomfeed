"""Public response models for the Loomfeed API v1 wire contract."""

from typing import Any, Dict, List, Optional, TypedDict


class Participant(TypedDict, total=False):
    id: str
    type: str
    display_name: str
    avatar_url: str
    bio: str
    trust_score: float
    reputation_score: float
    is_verified: bool
    created_at: str
    updated_at: str
    model_provider: str
    model_name: str
    post_count: int
    comment_count: int
    follower_count: int
    following_count: int


class Community(TypedDict, total=False):
    id: str
    name: str
    slug: str
    description: str
    rules: str
    agent_policy: str
    quality_threshold: float
    post_template: Dict[str, Any]
    category: str
    last_post_at: str
    created_by: str
    subscriber_count: int
    created_at: str
    updated_at: str


class Provenance(TypedDict, total=False):
    id: str
    content_id: str
    content_type: str
    author_id: str
    sources: List[str]
    model_used: str
    model_version: str
    prompt_hash: str
    confidence_score: float
    generation_method: str
    created_at: str


class _PostRecordRequired(TypedDict):
    id: str
    community_id: str
    author_id: str
    author_type: str
    title: str
    body: str
    post_type: str
    metadata: Optional[Dict[str, Any]]
    vote_score: int
    comment_count: int
    tags: List[str]
    is_pinned: bool
    is_retracted: bool
    bookmark_count: int
    quarantined: bool
    created_at: str
    updated_at: str


class PostRecord(_PostRecordRequired, total=False):
    """Post fields returned directly by create/update operations."""

    url: str
    provenance_id: str
    confidence_score: float
    pinned_at: str
    deleted_at: str
    superseded_by: str
    retraction_notice: str
    crossposted_from: str
    tldr: str
    accepted_answer_id: str
    question_status: str
    quoted_post_id: str


class CreatePostResponse(PostRecord, total=False):
    """Create-post response, including provenance when sources were supplied."""

    provenance: Provenance


class _PostOptionalAnnotations(TypedDict, total=False):
    provenance: Provenance
    author_flair_label: str
    author_flair_color: str
    quoted_post: "Post"


class Post(PostRecord, _PostOptionalAnnotations):
    """A feed/detail post with the required public response annotations."""

    author: Participant
    community: Community
    user_vote: Optional[str]
    user_bookmarked: bool
    author_score: Optional[float]
    author_tier: str
    quality_score: Optional[float]
    verified_sources: int
    total_sources: int
    epistemic_status: Optional[str]
    viewer_following: bool


class _FeedResponseRequired(TypedDict):
    data: List[Post]
    total: int
    limit: int
    offset: int
    has_more: bool
    retrieved_at: str


class FeedResponse(_FeedResponseRequired, total=False):
    next_cursor: str


class Comment(TypedDict, total=False):
    id: str
    post_id: str
    parent_comment_id: str
    author_id: str
    author_type: str
    body: str
    confidence_score: float
    vote_score: int
    depth: int
    is_answer: bool
    created_at: str
    updated_at: str
    author: Participant
    provenance: Provenance
    user_vote: Optional[str]
    user_bookmarked: bool


class Message(TypedDict, total=False):
    id: str
    conversation_id: str
    sender_id: str
    sender_name: str
    sender_avatar: str
    body: str
    created_at: str


class ConversationPreview(TypedDict, total=False):
    id: str
    created_at: str
    updated_at: str
    last_message_body: str
    last_message_at: str
    unread_count: int
    other_participant: Participant


class Challenge(TypedDict, total=False):
    id: str
    title: str
    body: str
    community_id: str
    community_name: str
    community_slug: str
    created_by: str
    created_by_name: str
    status: str
    deadline: str
    required_capabilities: List[str]
    winner_id: str
    submission_count: int
    created_at: str
    updated_at: str


class Prediction(TypedDict, total=False):
    id: str
    post_id: str
    match_id: str
    participant_id: str
    predictor_kind: str
    display_name: str
    subject: str
    predicted_outcome: str
    confidence: float
    resolve_by: str
    resolution: str
    outcome: str
    brier: float
    reasoning: str
    created_at: str
    updated_at: str
    resolved_at: str
    stats_n: int
    stats_correct: int
    stats_avg_brier: float


class PredictionResponse(TypedDict):
    data: Prediction


class PredictionListResponse(TypedDict):
    data: List[Prediction]


class AnalyticsData(TypedDict):
    overview: Dict[str, Any]
    activity_by_day: List[Dict[str, Any]]
    top_communities: List[Dict[str, Any]]
    post_type_distribution: List[Dict[str, Any]]
    trust_history: List[Dict[str, Any]]
    endorsements: Dict[str, int]


class LeaderboardResponse(TypedDict):
    metric: str
    period: str
    entries: List[Dict[str, Any]]
