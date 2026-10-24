"""Integrity checks for frozen study identities and evaluation boundaries."""
import hashlib,json,pathlib,unittest
P=pathlib.Path(__file__).parent
def read(name):return json.loads((P/name).read_text())
class EvidenceTest(unittest.TestCase):
    def test_locked_inputs_unchanged(self):
        for name,digest in read('evidence/retrieval-input-lock.json')['files'].items():
            self.assertEqual(hashlib.sha256((P/name).read_bytes()).hexdigest(),digest,name)
    def test_groups_do_not_cross_partitions(self):
        groups={}
        for row in read('evidence/split-v2.json')['entries']:
            groups.setdefault(row['duplicate_group'],set()).add(row['split'])
        self.assertTrue(all(len(s)==1 for s in groups.values()))
    def test_labels_keep_real_ids_and_hashes(self):
        for corpus,labels,count in [('evidence/corpus-450.json','ai-all-annotations.json',450),('evidence/corpus-kolobki.json','ai-kolobki-annotations.json',52)]:
            source={r['id']:r for r in read(corpus)['entries']};rows=read(labels)['entries']
            self.assertEqual(len(source),count);self.assertEqual(len(rows),count)
            for row in rows:
                for key in ['sha256','custom_emoji_id']:self.assertEqual(row[key],source[row['id']][key])
    def test_query_counts_and_seed_boundaries(self):
        queries=read('queries-frozen.json')['entries']
        for split,pos,neg in [('calibration',20,10),('test',70,20),('kolobki',20,5)]:
            subset=[q for q in queries if q['split']==split]
            self.assertEqual(sum(not q['no_match'] for q in subset),pos)
            self.assertEqual(sum(q['no_match'] for q in subset),neg)
            for q in subset:self.assertEqual(bool(q['seed_relevance']),not q['no_match'])
    def test_kolobki_qrels_are_explicit_and_exhaustive(self):
        ids={r['id'] for r in read('evidence/corpus-kolobki.json')['entries']}
        rows=read('kolobki-qrels.json')['entries'];self.assertEqual(len(rows),25)
        for row in rows:
            self.assertEqual(set(row['grades']),ids)
            self.assertTrue(set(row['grades'].values())<={0,1,2,3})
if __name__=='__main__':unittest.main()
