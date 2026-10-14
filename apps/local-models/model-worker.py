"""Tensor-only exception. Go owns authentication, scheduling and process lifetime."""
import contextlib
import base64
import io
import importlib.metadata
import hashlib
import stat
import json
import os
import pathlib
import sys
os.environ.update(HF_HUB_OFFLINE='1', TRANSFORMERS_OFFLINE='1', HF_HUB_DISABLE_TELEMETRY='1', TOKENIZERS_PARALLELISM='false')

def emit(value):
    sys.stdout.write(json.dumps(value, ensure_ascii=False, allow_nan=False) + '\n')
    sys.stdout.flush()

def bounded_tokens(text):
    if len(tokenizer(text, add_special_tokens=True)['input_ids']) > int(profile['context_tokens']):
        raise ValueError('input too long')

MEMORY_FIELDS = {
    'project': 'Named project', 'component': 'Named software component',
    'task': 'Task or goal explicitly stated in the text', 'resource': 'Server or other resource',
    'action': 'Action verb or requested operation', 'deadline': 'Deadline or time expression',
    'prohibition': 'Explicit negation or prohibition', 'exception': 'Explicit exception or allowed alternative',
    'condition': 'Condition that must hold', 'temporal_qualifier': 'Earlier or now qualifier',
}
MEMORY_RELATIONS = {
    'action_resource': ('action', 'resource', 'Action applies to resource'),
    'action_deadline': ('action', 'deadline', 'Action has deadline'),
    'action_exception': ('action', 'exception', 'Action has exception'),
    'project_component': ('project', 'component', 'Project contains component'),
}

def memory_schema(model):
    schema = model.create_schema().entities(MEMORY_FIELDS).relations(
        {key: value[2] for key, value in MEMORY_RELATIONS.items()})
    builder = schema.structure('constraint', mode='natural', anchor='action')
    for field in ('action', 'resource', 'deadline', 'prohibition', 'exception', 'condition', 'temporal_qualifier'):
        # Optional fields never require the decoder to invent missing content.
        builder.field(field, dtype='str', description=MEMORY_FIELDS[field],
                      cardinality='required_one' if field == 'action' else 'optional_one')
    return builder.build()

def memory_output(result):
    def span(label, item):
        return {'type': label, 'text': item['text'], 'start': item['start'],
                'end': item['end'], 'confidence': item['confidence']}
    spans = [span(label, item) for label, items in result.get('entities', {}).items() for item in items]
    relations = []
    for label, items in result.get('relation_extraction', {}).items():
        head, tail, _ = MEMORY_RELATIONS[label]
        for item in items:
            relations.append({'type': label, 'head': span(head, item['head']), 'tail': span(tail, item['tail'])})
    records = []
    for item in result.get('constraint', []):
        fields, unresolved = [], []
        for label, value in item.items():
            if label == 'confidence':
                # Upstream record-level score is metadata, not a schema field.
                continue
            if label not in MEMORY_FIELDS:
                raise ValueError('unexpected record field')
            if isinstance(value, list):
                if len(value) != 1:
                    unresolved.append(label)
                    continue
                value = value[0]
            if isinstance(value, dict) and all(key in value for key in ('text','start','end','confidence')):
                fields.append(span(label, value))
            else:
                unresolved.append(label)
        if any(field['type'] == 'action' for field in fields):
            records.append({'type': 'constraint', 'fields': fields, 'unresolved_fields': unresolved})
    return {'kind': 'spans', 'spans': spans, 'relations': relations, 'records': records}

# SHA-256 of every reviewed executable/config file in one immutable upstream
# revision. No runtime caller can supply code or choose another revision.
D1_REVIEW = {
 'audio.py':'c5ff09d52d6a079e79d0505c89dbd0582ba2bc9abf2b26595d8d5b1d0c347bdf',
 'encoder.py':'5f2e20319ee42a7367febf06c7653d7b3f9b28ef99a64cb9ec7739563dcfcffe',
 'modeling_d1.py':'2b71a5c909ec5d3aa6307378a41a0fd24af1e17d5c1c1f8d510582147051a686',
 'prompt.py':'a6b29a55ec8345f1fc1fdbfcc4b64d80d473dc4316f095a62c51cb6cce1194cf',
 'vision.py':'43ad71029f82e20fd26b97574627773d47639d7a3742c8b04fe5bf55f41c4c28',
 'config.json':'dfccae241822f81752426629c88c8b8d7efdd03dd19418b6645c604fad43d619',
 'tokenizer_config.json':'03efc74b752b899b6292ff7e607738be59e6f206fec9aca32bbc05e9184090fc',
}

def d1_exact_input(model, text, labels, limit, taxonomy=None):
    # Verify that the upstream encoder would serialize ALL state/options. Its
    # default per-option/right truncation is not an allowed Butler fallback.
    import importlib
    prompt = importlib.import_module(type(model).__module__.rsplit('.', 1)[0]+'.prompt')
    criteria = ({chr(65+i): taxonomy['labels'][label] for i,label in enumerate(labels)}
                if taxonomy else {label:'' for label in labels})
    q = prompt.as_question({'type':'choice','instructions':taxonomy['instructions'] if taxonomy else 'Classify the event.',
                            'criteria':criteria})
    tok = model.tokenizer
    enc = lambda s: tok(prompt.escape(s), add_special_tokens=False)['input_ids']
    ids = [tok.bos_token_id, tok.convert_tokens_to_ids(prompt.DELIM['state'])]
    ids += enc(prompt.serialize(text))
    ids += [tok.convert_tokens_to_ids(prompt.DELIM['q'])]+enc(q.instructions)
    for option in prompt.render_options(q):
        ids += [tok.convert_tokens_to_ids(prompt.DELIM['opt']),tok.convert_tokens_to_ids(prompt.MARKER)]
        ids += enc(' '+option)+[tok.convert_tokens_to_ids(prompt.DELIM['opt_end'])]
    ids += [tok.convert_tokens_to_ids(prompt.DELIM['decide'])]
    actual,_ = prompt.encode(tok,text,q,limit)
    if len(ids)>limit or ids!=actual:
        raise ValueError('decision input would be truncated')
    return {'category':{'type':'choice','instructions':q.instructions,'criteria':q.criteria}}

try:
    init = json.loads(sys.stdin.readline(65536))
    if init.get('op') != 'init':
        raise ValueError('invalid init')
    profile = init['profile']
    taxonomy = None
    if profile['preprocessing_version'] == 'butler-event-taxonomy-v2':
        raw = pathlib.Path(__file__).with_name('decision-schema-v2.json').read_bytes()
        if hashlib.sha256(raw).hexdigest() != 'ae9eeab71dfbeb835a28a8fa59806e75a4f13090225350b8b818a93a2c61763e':
            raise ValueError('taxonomy identity mismatch')
        taxonomy = json.loads(raw)
    runtimes = {
        'google/embeddinggemma-2': 'torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0',
        'Qwen/Qwen3-Reranker-0.6B': 'torch2.10.0+cu128/transformers5.19.0',
        'fastino/gliner2.5-multi-v1': 'torch2.10.0+cu128/transformers5.19.0/gliner2.1.0',
        'fastino/GLiNER2.5-multi-Decide': 'torch2.10.0+cu128/transformers5.19.0/gliner2.1.0',
        'LiquidAI/d1-omni-600M': 'torch2.10.0+cu128/transformers5.19.0/d1code02b55d7',
        'Qwen/Qwen3-ASR-0.6B': 'torch2.10.0+cu128/transformers4.57.6/qwen-asr0.0.6',
        'Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice': 'torch2.10.0+cu128/transformers4.57.3/qwen-tts0.1.1',
        'snakers4/silero-vad': 'torch2.10.0+cu128/silero-vad6.2.3',
    }
    name = profile['model']
    if runtimes.get(name) != profile['runtime_version']:
        raise ValueError('unsupported profile')
    revision = profile['revision']
    if len(revision) != 40 or any(c not in '0123456789abcdef' for c in revision):
        raise ValueError('invalid revision')
    path = pathlib.Path(os.environ['BUTLER_MODEL_ROOT']) / revision
    metadata = json.loads((path / 'butler-model.json').read_text())
    if any(metadata[k] != profile[k] for k in ('model', 'revision', 'runtime_version')):
        raise ValueError('model identity mismatch')
    if name in ('Qwen/Qwen3-ASR-0.6B','Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice','snakers4/silero-vad'):
        import importlib.util
        spec=importlib.util.spec_from_file_location('butler_voice',pathlib.Path(__file__).with_name('voice-worker.py'))
        voice=importlib.util.module_from_spec(spec);spec.loader.exec_module(voice)
        voice.run(init,path)
        sys.exit(0)
    with contextlib.redirect_stdout(sys.stderr):
        import torch
        import transformers
        if torch.__version__ != '2.10.0+cu128' or transformers.__version__ != '5.19.0':
            raise ValueError('library version mismatch')
        device = init['device']
        if device not in ('cpu', 'gpu'):
            raise ValueError('unsupported device')
        if device == 'gpu' and (not torch.cuda.is_available() or not torch.cuda.is_bf16_supported()):
            raise ValueError('BF16 GPU unavailable')
        dtype = torch.bfloat16 if device == 'gpu' else torch.float32
        target = 'cuda:0' if device == 'gpu' else 'cpu'
        torch.set_num_threads(int(os.environ.get('BUTLER_MODEL_CPU_THREADS', '4')))
        if name == 'google/embeddinggemma-2':
            from sentence_transformers import SentenceTransformer
            if importlib.metadata.version('sentence-transformers') != '6.1.0':
                raise ValueError('library version mismatch')
            model = SentenceTransformer(str(path), device=target,
                trust_remote_code=False, local_files_only=True,
                model_kwargs={'torch_dtype': dtype, 'use_safetensors': True},
                config_kwargs=({'vision_config': None, 'audio_config': None} if profile['preprocessing_version']=='embeddinggemma2-retrieval-v1' else {'audio_config': None}))
            model.max_seq_length = int(profile['context_tokens'])
            tokenizer = model.tokenizer
        elif name == 'Qwen/Qwen3-Reranker-0.6B':
            tokenizer = transformers.AutoTokenizer.from_pretrained(str(path), padding_side='left', local_files_only=True, trust_remote_code=False)
            model = transformers.AutoModelForCausalLM.from_pretrained(str(path), torch_dtype=dtype, use_safetensors=True, local_files_only=True, trust_remote_code=False).to(target)
            yes, no = tokenizer.convert_tokens_to_ids('yes'), tokenizer.convert_tokens_to_ids('no')
        elif name == 'LiquidAI/d1-omni-600M':
            if device!='cpu' or revision!='02b55d7076f15129e59ab3f94783f32c4b088674':
                raise ValueError('unsupported decision profile')
            for filename,digest in D1_REVIEW.items():
                file = path/filename
                if not stat.S_ISREG(file.lstat().st_mode) or file.stat().st_size>1024*1024 or hashlib.sha256(file.read_bytes()).hexdigest()!=digest:
                    raise ValueError('reviewed code mismatch')
            model = transformers.AutoModel.from_pretrained(str(path),trust_remote_code=True,
                local_files_only=True,use_safetensors=True,dtype=torch.float32).to('cpu')
            tokenizer = transformers.AutoTokenizer.from_pretrained(str(path),local_files_only=True,trust_remote_code=False)
            model.__dict__['tokenizer'] = tokenizer
            model.config.max_length = int(profile['context_tokens'])
        else:
            if device != 'cpu' or importlib.metadata.version('gliner2') != '2.1.0':
                raise ValueError('unsupported extraction device')
            from gliner2 import AutoExtractor
            model = AutoExtractor.from_pretrained(str(path), map_location='cpu', quantize=False, local_files_only=True, token=False)
            tokenizer = transformers.AutoTokenizer.from_pretrained(str(path), local_files_only=True, trust_remote_code=False)
            if profile['preprocessing_version'] in ('gliner2.5-classification-softmax-v1','butler-event-taxonomy-v2'):
                from gliner2.classification import Classifier, ClassificationSchema, ClassificationConfig
                # Reuse the already admitted model; never load a second checkpoint.
                classifier = Classifier(model).eval()
        model.eval()
    emit({'kind': 'ready', 'vram_mib': ((torch.cuda.memory_reserved() + 1048575) // 1048576) if device == 'gpu' else 0})
    for line in sys.stdin:
        if len(line) > 4 * 1024 * 1024:
            emit({'kind': 'error', 'code': 'invalid_input'})
            continue
        try:
            request = json.loads(line)
            with contextlib.redirect_stdout(sys.stderr), torch.inference_mode():
                op = request.get('op')
                if op == 'embed' and name == 'google/embeddinggemma-2':
                    texts = request['texts']
                    if not texts or len(texts) > int(profile['max_batch']):
                        raise ValueError('invalid batch')
                    for text in texts:
                        bounded_tokens(text)
                    embeddings = model.encode(texts, batch_size=len(texts), show_progress_bar=False, normalize_embeddings=False, convert_to_numpy=True)
                    output = {'kind': 'embeddings', 'embeddings': embeddings.tolist()}
                elif op == 'image' and name == 'google/embeddinggemma-2' and profile['preprocessing_version']=='embeddinggemma2-image-v1':
                    from PIL import Image
                    Image.MAX_IMAGE_PIXELS = 512 * 512
                    image = Image.open(io.BytesIO(base64.b64decode(request['content'], validate=True)))
                    if image.width > 512 or image.height > 512 or image.format not in ('PNG', 'JPEG'):
                        raise ValueError('image dimensions/format')
                    image = image.convert('RGB')
                    embeddings = model.encode([{'image': image}], show_progress_bar=False, normalize_embeddings=False, convert_to_numpy=True)
                    output = {'kind': 'embeddings', 'embeddings': embeddings.tolist()}
                elif op == 'rank' and name == 'Qwen/Qwen3-Reranker-0.6B':
                    texts = request['texts']
                    if not texts or len(texts) > int(profile['max_batch']):
                        raise ValueError('invalid batch')
                    pairs = [tokenizer.apply_chat_template([
                        {'role': 'query', 'content': request['query']},
                        {'role': 'document', 'content': document}], tokenize=False) for document in texts]
                    for text in pairs:
                        bounded_tokens(text)
                    inputs = tokenizer(pairs, padding=True, truncation=False, return_tensors='pt').to(target)
                    logits = model(**inputs).logits[:, -1, [no, yes]].float()
                    output = {'kind': 'scores', 'scores': torch.softmax(logits, dim=1)[:, 1].cpu().tolist()}
                elif op == 'classify' and name == 'LiquidAI/d1-omni-600M':
                    text, labels = request['text'],request['labels']
                    if taxonomy and set(labels) != set(taxonomy['labels']):
                        raise ValueError('taxonomy labels mismatch')
                    questions = d1_exact_input(model,text,labels,int(profile['context_tokens']),taxonomy)
                    result = model.system_one(text,questions)['answers']['category']['probabilities']
                    output = {'kind':'classes','labels':labels,'scores':[result[chr(65+i) if taxonomy else label] for i,label in enumerate(labels)]}
                elif op in ('extract', 'classify') and name in ('fastino/gliner2.5-multi-v1','fastino/GLiNER2.5-multi-Decide'):
                    text, labels = request['text'], request.get('labels') or []
                    bounded_tokens(text + ' ' + ' '.join(labels))
                    if op == 'extract':
                        if request.get('schema_version') == 'butler-memory-v1':
                            schema = memory_schema(model)
                            # Public upstream preprocessing checks the real schema
                            # plus text encoding, without truncation or fallback.
                            prepared = model.processor.collate_fn_inference(
                                [(text, schema)], max_len=None,
                                error_policy='raise', architecture='boundary')
                            if prepared.input_ids.shape[-1] > int(profile['context_tokens']):
                                raise ValueError('encoded schema plus text too long')
                            previous_strict = model.strict_extraction
                            try:
                                model.strict_extraction = True
                                result = model.extract(text, schema, threshold=0.5, include_confidence=True, include_spans=True, max_len=None)
                            finally:
                                model.strict_extraction = previous_strict
                            output = memory_output(result)
                        else:
                            result = model.extract_entities(text, labels, include_confidence=True, include_spans=True)
                            spans = []
                            for label, items in result['entities'].items():
                                for item in items:
                                    spans.append({'type': label, 'text': item['text'], 'start': item['start'], 'end': item['end'], 'confidence': item['confidence']})
                            output = {'kind': 'spans', 'spans': spans}
                    else:
                        if profile['preprocessing_version'] in ('gliner2.5-classification-softmax-v1','butler-event-taxonomy-v2'):
                            if taxonomy and set(labels) != set(taxonomy['labels']):
                                raise ValueError('taxonomy labels mismatch')
                            compiled = classifier.compile_schema(
                                ClassificationSchema().single('category', taxonomy['labels'] if taxonomy else labels,
                                    activation='softmax', instruction=taxonomy['instructions'] if taxonomy else None))
                            prepared = model.processor.collate_fn_inference(
                                [(text, compiled.build())], max_len=None,
                                error_policy='raise', architecture='boundary')
                            if prepared.input_ids.shape[-1] > int(profile['context_tokens']):
                                raise ValueError('classification schema plus text too long')
                            scores = classifier.score(text, compiled, config=ClassificationConfig(max_len=None))
                            if set(scores.tasks['category']) != set(labels):
                                raise ValueError('incomplete classification scores')
                            output = {'kind': 'classes', 'labels': labels,
                                      'scores': [scores.probability('category', label) for label in labels]}
                        else:
                            schema = model.create_schema().classification('category',labels).build()
                            prepared = model.processor.collate_fn_inference([(text,schema)],max_len=None,
                                error_policy='raise',architecture='boundary')
                            if prepared.input_ids.shape[-1]>int(profile['context_tokens']):
                                raise ValueError('classification schema plus text too long')
                            result = model.classify_text(text, {'category': labels}, include_confidence=True,max_len=None)
                            category = result.get('category')
                            output = {'kind': 'classes', 'labels': [] if category is None else [category['label']], 'scores': [] if category is None else [category['confidence']]}
                else:
                    raise ValueError('unsupported operation')
            emit(output)
        except torch.OutOfMemoryError:
            emit({'kind': 'error', 'code': 'out_of_memory'})
        except (ValueError, KeyError, TypeError):
            emit({'kind': 'error', 'code': 'invalid_input'})
        except Exception:
            emit({'kind': 'error', 'code': 'model_unavailable'})
except Exception as error:
    emit({'kind': 'error', 'code': 'out_of_memory' if type(error).__name__ == 'OutOfMemoryError' else 'model_unavailable'})
    sys.exit(1)
