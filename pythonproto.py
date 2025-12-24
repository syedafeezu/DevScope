import os
import sys
import struct
import time
import argparse
import re
import math
from collections import defaultdict
from datetime import datetime

# --- Constants & Config ---
INDEX_DIR = ".devscope"
DOCS_FILE = "docs.bin"
INDEX_FILE = "index.bin"
LEXICON_FILE = "lexicon.bin"

# Meta Filtering
META_NONE = 0
META_IN_FILENAME = 1 << 0
META_IN_FUNCNAME = 1 << 1
META_LOG_ERROR = 1 << 2
META_LOG_WARN = 1 << 3

DOC_TYPE_CODE = 0
DOC_TYPE_LOG = 1

# --- Tokenizer ---
RE_IDENTIFIER = re.compile(r'[a-zA-Z_][a-zA-Z0-9_]*')
RE_FUNC_DEF = re.compile(r'(func|def|function|class|struct)\s+([a-zA-Z_][a-zA-Z0-9_]*)')

def parse_timestamp(line):
    # Try generic ISO-like: 2025-12-20T10:00:00
    if len(line) < 19:
        return 0
    chunk = line[:19].replace(" ", "T")
    try:
        # crude parse
        dt = datetime.strptime(chunk, "%Y-%m-%dT%H:%M:%S")
        return int(dt.timestamp())
    except ValueError:
        return 0

def tokenize(path, doc_type):
    tokens = []
    min_ts, max_ts = 0, 0
    
    try:
        with open(path, 'r', encoding='utf-8', errors='ignore') as f:
            line_num = 0
            for line in f:
                line_num += 1
                line_content = line
                
                meta = META_NONE
                
                if doc_type == DOC_TYPE_LOG:
                    ts = parse_timestamp(line)
                    if ts > 0:
                        if min_ts == 0 or ts < min_ts: min_ts = ts
                        if ts > max_ts: max_ts = ts
                    
                    upper = line.upper()
                    if "ERROR" in upper: meta |= META_LOG_ERROR
                    elif "WARN" in upper: meta |= META_LOG_WARN
                
                else: # CODE
                    # Function detection
                    m = RE_FUNC_DEF.search(line)
                    func_name = m.group(2) if m else None
                
                # Extract terms
                for m in RE_IDENTIFIER.finditer(line):
                    term = m.group(0)
                    term_meta = meta
                    
                    if doc_type == DOC_TYPE_CODE and func_name and term == func_name:
                        term_meta |= META_IN_FUNCNAME
                        
                    tokens.append((term, line_num, term_meta))
                    
    except Exception as e:
        print(f"Warning: Failed to read {path}: {e}")
        return [], 0, 0
        
    return tokens, min_ts, max_ts

# --- Indexer ---

def write_doc(f, doc_id, doc_type, path, t_min, t_max):
    # DocID(4), Type(1), PathLen(2), Path(N), TMin(8), TMax(8)
    path_bytes = path.encode('utf-8')
    data = struct.pack('<IBH', doc_id, doc_type, len(path_bytes))
    f.write(data)
    f.write(path_bytes)
    f.write(struct.pack('<qq', t_min, t_max))

def index(target_path):
    if not os.path.exists(INDEX_DIR):
        os.makedirs(INDEX_DIR)
        
    start_time = time.time()
    
    # In-memory index: term -> doc_id -> (freq, [positions], meta_mask)
    mem_index = defaultdict(lambda: defaultdict(lambda: {'freq': 0, 'pos': [], 'meta': 0}))
    
    doc_id_counter = 1
    
    with open(os.path.join(INDEX_DIR, DOCS_FILE), 'wb') as f_docs:
        for root, dirs, files in os.walk(target_path):
            if '.git' in dirs: dirs.remove('.git')
            if 'node_modules' in dirs: dirs.remove('node_modules')
            if '.devscope' in dirs: dirs.remove('.devscope')
            
            for file in files:
                ext = os.path.splitext(file)[1].lower()
                doc_type = -1
                if ext in ['.log']:
                    doc_type = DOC_TYPE_LOG
                elif ext in ['.go', '.py', '.js', '.ts', '.c', '.cpp', '.java', '.md', '.txt', '.json']:
                    doc_type = DOC_TYPE_CODE
                
                if doc_type == -1: continue
                
                path = os.path.join(root, file)
                
                # Tokenize
                tokens, min_ts, max_ts = tokenize(path, doc_type)
                if not tokens and doc_type == DOC_TYPE_CODE:
                    continue # Skip empty binary files purely identified by extension
                
                # Write Doc
                write_doc(f_docs, doc_id_counter, doc_type, path, min_ts, max_ts)
                
                # meaningful tokens only?
                # Update mem index
                for term, pos, meta in tokens:
                    entry = mem_index[term][doc_id_counter]
                    entry['freq'] += 1
                    entry['pos'].append(pos)
                    entry['meta'] |= meta
                
                print(f"\rIndexed {doc_id_counter} files...", end="")
                doc_id_counter += 1
                
    print(f"\nIndexing complete in {time.time() - start_time:.2f}s. Saving index...")
    
    # Write Index & Lexicon
    with open(os.path.join(INDEX_DIR, INDEX_FILE), 'wb') as f_idx, \
         open(os.path.join(INDEX_DIR, LEXICON_FILE), 'wb') as f_lex:
         
        terms = sorted(mem_index.keys())
        offset_counter = 0
        
        for term in terms:
            doc_map = mem_index[term]
            doc_ids = sorted(doc_map.keys())
            
            start_offset = offset_counter
            
            for did in doc_ids:
                entry = doc_map[did]
                # DocID(4), Freq(4), Meta(1), PosCount(4), Positions(4*N)
                header = struct.pack('<IIBI', did, entry['freq'], entry['meta'], len(entry['pos']))
                f_idx.write(header)
                offset_counter += 13
                
                for p in entry['pos']:
                    f_idx.write(struct.pack('<I', p))
                    offset_counter += 4
            
            # Lexicon: TermLen(1), Term(N), DocFreq(4), Offset(8), Padding(4)
            term_bytes = term.encode('utf-8')[:255]
            f_lex.write(struct.pack('<B', len(term_bytes)))
            f_lex.write(term_bytes)
            f_lex.write(struct.pack('<IQI', len(doc_ids), start_offset, 0))

# --- Searcher ---

def read_docs(path):
    docs = {}
    with open(path, 'rb') as f:
        while True:
            # Header: 4+1+2 = 7
            header = f.read(7)
            if not header: break
            doc_id, dtype, path_len = struct.unpack('<IBH', header)
            path_bytes = f.read(path_len)
            meta = f.read(16)
            doc = {
                'id': doc_id,
                'path': path_bytes.decode('utf-8'),
                'type': dtype,
            }
            docs[doc_id] = doc
    return docs

def read_lexicon(path):
    lex = {}
    with open(path, 'rb') as f:
        while True:
            lb = f.read(1)
            if not lb: break
            length = struct.unpack('<B', lb)[0]
            term = f.read(length).decode('utf-8')
            meta = f.read(16)
            df, offset, _ = struct.unpack('<IQI', meta)
            lex[term] = {'df': df, 'offset': offset}
    return lex

def get_postings(f_idx, offset, doc_freq):
    f_idx.seek(offset)
    postings = []
    for _ in range(doc_freq):
        header = f_idx.read(13)
        doc_id, freq, meta, pos_count = struct.unpack('<IIBI', header)
        
        pos_data = f_idx.read(4 * pos_count)
        # Just skip reading positions into array if not needed for snippet yet, but we read to advance
        # Actually simple array is fine
        positions = []
        # for i in range(pos_count):
        #    positions.append(struct.unpack_from('<I', pos_data, i*4)[0])
        # optimization: don't parse positions unless needed?
        # We need positions for phrase search (not impl) or snippet line.
        # Let's just store first position for now to save time
        first_pos = 0
        if pos_count > 0:
            first_pos = struct.unpack_from('<I', pos_data, 0)[0]
            
        postings.append({'doc_id': doc_id, 'freq': freq, 'meta': meta, 'line': first_pos})
    return postings

def search(query_str):
    if not os.path.exists(INDEX_DIR):
        print("Index not found.")
        return

    docs = read_docs(os.path.join(INDEX_DIR, DOCS_FILE))
    lexicon = read_lexicon(os.path.join(INDEX_DIR, LEXICON_FILE))
    total_docs = len(docs)
    
    parts = query_str.split()
    filters = {'ext': None, 'level': None}
    terms = []
    
    for p in parts:
        if p.startswith("ext:"): filters['ext'] = p[4:].lower()
        elif p.startswith("level:"): filters['level'] = p[6:].upper()
        else: terms.append(p)
    
    if not terms: return
    
    scores = defaultdict(float)
    matches = defaultdict(int)
    
    with open(os.path.join(INDEX_DIR, INDEX_FILE), 'rb') as f_idx:
        for term in terms:
            if term not in lexicon: continue
            entry = lexicon[term]
            
            postings = get_postings(f_idx, entry['offset'], entry['df'])
            idf = math.log10(total_docs / (entry['df'] + 1))
            
            for p in postings:
                doc = docs[p['doc_id']]
                
                # Filters
                if filters['ext'] and not doc['path'].lower().endswith(filters['ext']): continue
                
                if filters['level'] == 'ERROR':
                     if not (p['meta'] & META_LOG_ERROR): continue
                
                # Scoring
                score = p['freq'] * idf
                if p['meta'] & META_IN_FILENAME: score += 5
                if p['meta'] & META_IN_FUNCNAME: score += 3
                if p['meta'] & META_LOG_ERROR: score += 2
                
                scores[p['doc_id']] += score
                matches[p['doc_id']] += 1

    # AND logic
    results = []
    for doc_id, count in matches.items():
        if count == len(terms):
            results.append((doc_id, scores[doc_id]))
            
    results.sort(key=lambda x: x[1], reverse=True)
    
    print(f"\nFound {len(results)} results.\n")
    for doc_id, score in results[:10]:
        doc = docs[doc_id]
        print(f"{doc['path']} (Score: {score:.2f})")
        # Snippet
        try:
            with open(doc['path'], 'r', errors='ignore') as f:
                lines = f.readlines()
                # Find line with term
                for i, line in enumerate(lines):
                    if any(t in line for t in terms):
                        print(f"  {i+1}: {line.strip()[:200]}")
                        break
        except:
            pass
        print()

# --- Main ---
if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="DevScope")
    subparsers = parser.add_subparsers(dest='command')
    
    idx_parser = subparsers.add_parser('index')
    idx_parser.add_argument('path')
    
    search_parser = subparsers.add_parser('search')
    search_parser.add_argument('query', nargs='+')
    
    args = parser.parse_args()
    
    if args.command == 'index':
        index(args.path)
    elif args.command == 'search':
        search(" ".join(args.query))
    else:
        parser.print_help()
