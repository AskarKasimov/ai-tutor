"""Load existing backend configuration without exposing credentials in artifacts."""
import argparse
import os
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--env-file', type=Path, required=True)
parser.add_argument('--binary', type=Path, required=True)
parser.add_argument('--limit', default='0')
parser.add_argument('--ids', default='')
args = parser.parse_args()
config = {}
for line in args.env_file.read_text(encoding='utf-8-sig').splitlines():
    if '=' in line and not line.lstrip().startswith('#'):
        key, value = line.split('=', 1)
        config[key.strip()] = value.strip().strip('\"\'')
env = os.environ.copy()
env['QA_TRANSLATOR_URL'] = config.get('BACKEND_TRANSLATOR_BASE_URL') or config.get('BACKEND_TUTOR_BASE_URL') or config.get('BACKEND_ASSESSMENT_BASE_URL', '')
env['QA_TRANSLATOR_MODEL'] = config.get('BACKEND_TRANSLATOR_MODEL') or config.get('BACKEND_TUTOR_MODEL') or config.get('BACKEND_ASSESSMENT_MODEL', '')
env['QA_TRANSLATOR_KEY'] = config.get('BACKEND_TRANSLATOR_API_KEY') or config.get('BACKEND_LLM_API_KEY', '')
if not env['QA_TRANSLATOR_URL'] or not env['QA_TRANSLATOR_MODEL']:
    raise SystemExit('Translator endpoint/model not configured')
root = Path(__file__).resolve().parent
raise SystemExit(subprocess.call([
    str(args.binary.resolve()), '--input', str(root / 'GIC_RU_translation_partial_v1/course_graph.ru-draft.v1.1.json'),
    '--output', str(root / 'vache_results.jsonl'), '--limit', args.limit, '--ids', args.ids,
], env=env))
