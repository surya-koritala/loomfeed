import json
from pathlib import Path
from typing import get_args, get_type_hints
from unittest import TestCase
from unittest.mock import Mock

import requests

from loomfeed import SDK_CONTRACT_VERSION, LoomfeedClient
from loomfeed.types import Comment, CreatePostResponse, FeedResponse, Post


CONTRACT_DIR = Path(__file__).parents[2] / "contracts" / "v1"


def load_fixture(name):
    return json.loads((CONTRACT_DIR / name).read_text(encoding="utf-8"))


class FakeResponse:
    def __init__(self, status_code, payload):
        self.status_code = status_code
        self._payload = payload
        self.reason = "Not Found" if status_code == 404 else "OK"

    def json(self):
        return self._payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.HTTPError(
                f"{self.status_code} Client Error: {self.reason}",
                response=self,
            )


class LoomfeedClientTests(TestCase):
    def test_public_models_mark_required_optional_and_nullable_fields(self):
        self.assertTrue(
            {"data", "total", "limit", "offset", "has_more", "retrieved_at"}
            <= FeedResponse.__required_keys__
        )
        self.assertIn("next_cursor", FeedResponse.__optional_keys__)
        self.assertTrue(
            {"metadata", "user_vote", "author_score", "quality_score", "epistemic_status"}
            <= Post.__required_keys__
        )
        self.assertIn("provenance", Post.__optional_keys__)
        self.assertIn("provenance", CreatePostResponse.__optional_keys__)

        hints = get_type_hints(Post)
        for field in (
            "metadata",
            "user_vote",
            "author_score",
            "quality_score",
            "epistemic_status",
        ):
            self.assertIn(type(None), get_args(hints[field]))
        self.assertIn(type(None), get_args(get_type_hints(Comment)["user_vote"]))

    def test_feed_uses_v1_envelope_and_wire_casing(self):
        client = LoomfeedClient(
            base_url="https://loomfeed.test/",
            api_key="ak_contract",
            timeout=12,
        )
        client._session.request = Mock(return_value=FakeResponse(200, load_fixture("feed.json")))

        feed = client.get_feed(sort="new", limit=5)

        self.assertEqual(SDK_CONTRACT_VERSION, "v1")
        self.assertEqual(feed["total"], 1)
        self.assertEqual(feed["data"][0]["community_id"], "community-1")
        self.assertEqual(feed["data"][0]["vote_score"], 7)
        client._session.request.assert_called_once_with(
            "GET",
            "https://loomfeed.test/api/v1/feed",
            params={"sort": "new", "limit": 5, "offset": 0},
            timeout=12,
        )
        self.assertEqual(client._session.headers["X-API-Key"], "ak_contract")

    def test_analytics_uses_live_route(self):
        client = LoomfeedClient(base_url="https://loomfeed.test", token="jwt_contract")
        client._session.request = Mock(
            return_value=FakeResponse(200, load_fixture("analytics.json"))
        )

        analytics = client.get_analytics("agent-1")

        self.assertEqual(analytics["overview"]["total_posts"], 10)
        client._session.request.assert_called_once_with(
            "GET",
            "https://loomfeed.test/api/v1/agent-profile/agent-1/analytics",
            params=None,
            timeout=30,
        )
        self.assertEqual(client._session.headers["Authorization"], "Bearer jwt_contract")

    def test_create_post_sends_wire_casing(self):
        client = LoomfeedClient(base_url="https://loomfeed.test")
        client._session.request = Mock(
            return_value=FakeResponse(
                201,
                {
                    **load_fixture("feed.json")["data"][0],
                    "provenance": {
                        "id": "provenance-1",
                        "sources": ["https://example.com/source"],
                        "confidence_score": 0.9,
                        "generation_method": "original",
                    },
                },
            )
        )

        post = client.create_post(
            community_id="community-1",
            title="Contract post",
            body="Body",
            confidence_score=0.9,
            generation_method="synthesis",
            sources=["https://example.com/source"],
        )

        self.assertEqual(post["vote_score"], 7)
        self.assertEqual(post["provenance"]["sources"], ["https://example.com/source"])
        request = client._session.request.call_args
        self.assertEqual(request.args, ("POST", "https://loomfeed.test/api/v1/posts"))
        self.assertEqual(request.kwargs["json"]["community_id"], "community-1")
        self.assertEqual(request.kwargs["json"]["confidence_score"], 0.9)
        self.assertNotIn("generation_method", request.kwargs["json"])

    def test_http_errors_preserve_the_response(self):
        response = FakeResponse(404, load_fixture("error.json"))
        client = LoomfeedClient(base_url="https://loomfeed.test")
        client._session.request = Mock(return_value=response)

        with self.assertRaises(requests.HTTPError) as raised:
            client.get_post("missing")

        self.assertIs(raised.exception.response, response)
        self.assertEqual(raised.exception.response.json()["error"], "post not found")
