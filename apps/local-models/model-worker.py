"""Tensor-only exception. Go owns authentication, scheduling and process lifetime."""
import contextlib
import base64
import io
import importlib.metadata
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

try:
    init = json.loads(sys.stdin.readline(65536))
    if init.get('op') != 'init':
        raise ValueError('invalid init')
    profile = init['profile']
    runtimes = {
        'google/embeddinggemma-2': 'torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0',
        'Qwen/Qwen3-Reranker-0.6B': 'torch2.10.0+cu128/transformers5.19.0',
        'fastino/gliner2.5-multi-v1': 'torch2.10.0+cu128/transformers5.19.0/gliner2.1.0',
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
        else:
            if device != 'cpu' or importlib.metadata.version('gliner2') != '2.1.0':
                raise ValueError('unsupported extraction device')
            from gliner2 import AutoExtractor
            model = AutoExtractor.from_pretrained(str(path), map_location='cpu', quantize=False, local_files_only=True, token=False)
            tokenizer = transformers.AutoTokenizer.from_pretrained(str(path), local_files_only=True, trust_remote_code=False)
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
                elif op in ('extract', 'classify') and name == 'fastino/gliner2.5-multi-v1':
                    text, labels = request['text'], request['labels']
                    bounded_tokens(text + ' ' + ' '.join(labels))
                    if op == 'extract':
                        if request.get('schema_version') == 'butler-memory-v1':
                            schema = memory_schema(model)
                            bounded_tokens(text + ' ' + ' '.join(MEMORY_FIELDS) + ' ' + ' '.join(MEMORY_RELATIONS))
                            output = memory_output(model.extract(text, schema, threshold=0.5, include_confidence=True, include_spans=True))
                        else:
                            result = model.extract_entities(text, labels, include_confidence=True, include_spans=True)
                            spans = []
                            for label, items in result['entities'].items():
                                for item in items:
                                    spans.append({'type': label, 'text': item['text'], 'start': item['start'], 'end': item['end'], 'confidence': item['confidence']})
                            output = {'kind': 'spans', 'spans': spans}
                    else:
                        result = model.classify_text(text, {'category': labels}, include_confidence=True)
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
