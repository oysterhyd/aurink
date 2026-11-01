import os, subprocess, sys
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer
src, out = sys.argv[1], sys.argv[2]
os.makedirs(out, exist_ok=True)

# GB2312 level 1 and 2, then ASCII, Latin-1 and CJK punctuation.
def gb(lo, hi):
    s = set()
    for a in range(lo, hi + 1):
        for b in range(0xA1, 0xFF):
            try:
                s.add(bytes([a, b]).decode('gb2312'))
            except UnicodeDecodeError:
                pass
    return s
l1, l2 = gb(0xB0, 0xD7), gb(0xD8, 0xF7)
base = {chr(c) for c in range(0x20, 0x100)} | {chr(c) for c in range(0x2010, 0x2040)} | {chr(c) for c in range(0x3000, 0x3020)} | {chr(c) for c in range(0xFF00, 0xFF66)}
print('level1', len(l1), 'level2', len(l2))

def subset(path, chars, name, flavor=None):
    u = os.path.join(out, 'u.txt')
    open(u, 'w', encoding='utf8').write(''.join(sorted(chars)))
    dst = os.path.join(out, name)
    cmd = [sys.executable, '-I', '-m', 'fontTools.subset', path, '--text-file=' + u, '--output-file=' + dst, '--layout-features=*']
    if flavor:
        cmd.append('--flavor=' + flavor)
    subprocess.check_call(cmd)
    print(f'{name}: {os.path.getsize(dst):,}')
    return dst

noto = os.path.join(src, 'NotoSerifSC[wght].ttf')
f = TTFont(noto)
print('Noto axes', [(a.axisTag, a.minValue, a.maxValue) for a in f['fvar'].axes], 'glyphs', f['maxp'].numGlyphs)
for wname, limit in [('500', {'wght': 500}), ('700', {'wght': 700}), ('var500-700', {'wght': (500, 700)})]:
    v = TTFont(noto)
    instancer.instantiateVariableFont(v, limit, inplace=True)
    p = os.path.join(out, f'tmp-{wname}.ttf')
    v.save(p)
    subset(p, base | l1, f'noto-l1-{wname}.ttf')
    subset(p, base | l1 | l2, f'noto-l1l2-{wname}.ttf')
    os.remove(p)
os.remove(os.path.join(out, 'u.txt'))
