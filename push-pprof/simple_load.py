#!/usr/bin/env python3

import asyncio
import aiohttp
import time
import sys
import argparse
from datetime import datetime, timezone

async def worker(session, url):
    while True:
        request_start = datetime.now(timezone.utc)
        try:
            async with session.get(url) as response:
                await response.read()

        except asyncio.TimeoutError:
            print(f"[{datetime.now(timezone.utc).isoformat()}] Request started at {request_start.isoformat()} - Request error: timeout")
        except Exception as e:
            print(f"[{datetime.now(timezone.utc).isoformat()}] Request started at {request_start.isoformat()} - Request error: {e}")


async def main():
    parser = argparse.ArgumentParser(description='Simple HTTP load generator')
    parser.add_argument('--url', default='https://profiles-prod-008.grafana.net', 
                        help='Target URL (default: https://profiles-prod-008.grafana.net)')
    
    args = parser.parse_args()
    
    num_workers = 1000
    timeout = 60.0
    
    print(f"Starting load test:")
    print(f"  URL: {args.url}")
    print()
    
    connector = aiohttp.TCPConnector(limit=0, limit_per_host=0)
    timeout_config = aiohttp.ClientTimeout(total=timeout)
    
    async with aiohttp.ClientSession(connector=connector, timeout=timeout_config) as session:
        workers = [asyncio.create_task(worker(session, args.url)) for _ in range(num_workers)]
        await asyncio.gather(*workers)

if __name__ == '__main__':
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\nStopped by user")
        sys.exit(0)