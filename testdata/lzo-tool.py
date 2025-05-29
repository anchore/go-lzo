#!/usr/bin/env python3
import sys
import lzo
import argparse

def main():
    parser = argparse.ArgumentParser(description='Raw LZO compression/decompression wrapper')
    parser.add_argument('-c', '--stdout', action='store_true', help='write to stdout')
    parser.add_argument('-d', '--decompress', action='store_true', help='decompress')
    parser.add_argument('-t', '--test', action='store_true', help='test compressed file')
    parser.add_argument('file', nargs='?', help='input file (default: stdin)')

    args = parser.parse_args()

    try:
        # read input...
        if args.file:
            with open(args.file, 'rb') as f:
                data = f.read()
        else:
            data = sys.stdin.buffer.read()

        if args.decompress or args.test:
            # decompress...
            try:
                result = lzo.decompress(data)
                if args.test:
                    print(f"OK: decompressed {len(data)} -> {len(result)} bytes", file=sys.stderr)
                    sys.exit(0)
                else:
                    sys.stdout.buffer.write(result)
            except Exception as e:
                print(f"Error decompressing: {e}", file=sys.stderr)
                sys.exit(1)
        else:
            # compress...
            # note: there is a f0000000 ~ish header in the front of the compressed data. The C code looks like it
            # should take header=False as a parameter, but it doesn't seem to work.
            # TODO: figure out how to detect and remove the header to use this script...
            result = lzo.compress(data)
            sys.stdout.buffer.write(result)

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == '__main__':
    main()
