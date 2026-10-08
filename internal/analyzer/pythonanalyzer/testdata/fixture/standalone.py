from pathlib import Path
Path(__file__).with_name('executed').write_text('bad')

def standalone():
    pass
