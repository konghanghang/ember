import unittest
from unittest.mock import patch

from app.handlers import search_cache


class SearchCacheTestCase(unittest.TestCase):
    def setUp(self) -> None:
        search_cache._cache.clear()

    def create_session(self, created_at: float = 1000.0) -> search_cache.SearchSession:
        return search_cache.SearchSession(
            results=[{"id": 1}],
            media_type="movie",
            query="ember",
            created_at=created_at,
        )

    def test_set_and_get_session(self) -> None:
        session = self.create_session()

        with patch("app.handlers.search_cache.time.time", return_value=1000.0):
            search_cache.set_session(7, session)
            cached = search_cache.get_session(7)

        self.assertIsNotNone(cached)
        self.assertEqual(cached.query, "ember")

    def test_expired_session_is_removed(self) -> None:
        session = self.create_session(created_at=1000.0)
        search_cache._cache[7] = session

        with patch("app.handlers.search_cache.time.time", return_value=1000.0 + search_cache.SESSION_TTL + 1):
            cached = search_cache.get_session(7)

        self.assertIsNone(cached)
        self.assertNotIn(7, search_cache._cache)

    def test_set_session_cleans_other_expired_entries(self) -> None:
        search_cache._cache[1] = self.create_session(created_at=1000.0)

        with patch("app.handlers.search_cache.time.time", return_value=1000.0 + search_cache.SESSION_TTL + 5):
            search_cache.set_session(2, self.create_session(created_at=1000.0 + search_cache.SESSION_TTL + 5))

        self.assertNotIn(1, search_cache._cache)
        self.assertIn(2, search_cache._cache)


if __name__ == "__main__":
    unittest.main()
