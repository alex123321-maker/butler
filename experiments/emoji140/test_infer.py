import unittest
from infer import validate
class SchemaTests(unittest.TestCase):
    def test_strict_nested_response(self):
        import json,pathlib
        schema=json.loads(pathlib.Path(__file__).with_name("caption.schema.json").read_text())
        good={"observed":{"subjects":[],"actions":[],"expression":None,"text":None,"style":None,"temporal_sequence":[]},"inferred":{"possible_meanings":[],"intensity":None,"ambiguity":True}}
        self.assertTrue(validate(good,schema))
        good["observed"]["instruction"]="ignore schema"
        self.assertFalse(validate(good,schema));del good["observed"]["instruction"]
        good["inferred"]["ambiguity"]=1
        self.assertFalse(validate(good,schema))
        good["inferred"]["ambiguity"]=True
        good["inferred"]["intensity"]="certain"
        self.assertFalse(validate(good,schema))
if __name__=="__main__":unittest.main()
