import unittest
from score_retrieval import success,mrr,ndcg,select_threshold,wilson
class MetricsTest(unittest.TestCase):
    def test_related_but_unsuitable_is_not_success(self):
        ranking=[{'id':'related'},{'id':'good'}];rel={'related':1,'good':3}
        self.assertEqual(success(ranking,rel,1),0);self.assertEqual(mrr(ranking,rel),.5)
        self.assertLess(ndcg(ranking,rel),1)
    def test_no_relevance_has_no_ndcg(self):
        self.assertIsNone(ndcg([{'id':'a'}],{'a':0}))
    def test_threshold_ties_keep_false_accept_bound(self):
        rows=[{'score':.5,'no_match':True} for _ in range(10)]+[{'score':.5,'no_match':False} for _ in range(20)]
        self.assertGreater(select_threshold(rows),.5)
    def test_threshold_can_keep_one_false_accept(self):
        rows=[{'score':.9,'no_match':True}]+[{'score':.2,'no_match':True} for _ in range(9)]+[{'score':.8,'no_match':False} for _ in range(20)]
        self.assertEqual(select_threshold(rows),.8)
    def test_zero_observed_errors_still_uncertain(self):
        self.assertGreater(wilson(0,20)['wilson_95'][1],.1)
if __name__=='__main__':unittest.main()
