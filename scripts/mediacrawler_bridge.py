"""Small on-demand adapter for an explicitly enabled, existing Chrome session."""
import argparse
import asyncio
import json
import logging
import os
import sys
import traceback
from pathlib import Path


def tieba_blocks(content):
    if isinstance(content, str):
        return [{"type": "text", "text": content}]
    blocks = []
    for item in content or []:
        if not isinstance(item, dict):
            continue
        if str(item.get("type")) == "3":
            image = next((item.get(k) for k in ("origin_src", "big_cdn_src", "big_src", "cdn_src", "src") if item.get(k)), "")
            if image:
                blocks.append({"type": "image", "url": image})
        else:
            text = item.get("text") or item.get("c")
            if text:
                blocks.append({"type": "text", "text": str(text)})
    return blocks


async def collect(args):
    from playwright.async_api import async_playwright
    import config
    config.ENABLE_IP_PROXY = False
    posts, comments = [], []
    search_count, has_more = 0, False
    async with async_playwright() as pw:
        browser = await pw.chromium.connect_over_cdp(args.cdp, timeout=15000)
        if not browser.contexts:
            raise RuntimeError("browser")
        context = browser.contexts[0]
        page = await context.new_page()
        try:
            if args.source == "xiaohongshu":
                from media_platform.xhs.core import XiaoHongShuCrawler
                from store import xhs as store
                from media_platform.xhs.help import get_search_id

                class MemoryStore:
                    async def store_content(self, row):
                        row.pop("xsec_token", None)
                        row["note_url"] = "https://www.xiaohongshu.com/explore/" + str(row["note_id"])
                        posts.append(row)

                    async def store_comment(self, row):
                        comments.append(row)

                store.XhsStoreFactory.create_store = staticmethod(lambda: MemoryStore())
                crawler = XiaoHongShuCrawler()
                crawler.browser_context, crawler.context_page = context, page
                await page.goto(crawler.index_url, wait_until="domcontentloaded", timeout=25000)
                client = await crawler.create_xhs_client(None)
                if not await client.pong():
                    raise RuntimeError("session")
                # The platform expects its normal 20-result search page. Split
                # it into small local batches without skipping other results.
                batches = (20 + args.limit - 1) // args.limit
                upstream_page = (args.page - 1) // batches + 1
                offset = ((args.page - 1) % batches) * args.limit
                results = await client.get_note_by_keyword(args.keyword, search_id=get_search_id(), page=upstream_page)
                items = [i for i in results.get("items", []) if i.get("id") and i.get("model_type") not in ("rec_query", "hot_query")]
                search_count = len(items)
                has_more = offset + args.limit < len(items) or bool(results.get("has_more"))
                for item in items[offset:offset + args.limit]:
                    note_id, token = item["id"], item.get("xsec_token", "")
                    try:
                        note = await client.get_note_by_id(note_id, "pc_search", token)
                    except Exception:
                        continue
                    if not note:
                        continue
                    note["note_id"] = note_id
                    await store.update_xhs_note(note)
                    # Preserve every available original image, not one joined URL.
                    posts[-1]["image_list"] = [i.get("url_default") or i.get("url") or next((v.get("url") for v in i.get("info_list", []) if v.get("url")), "") for i in note.get("image_list", [])]
                    await asyncio.sleep(2)
                    try:
                        response = await client.get_note_comments(note_id, token)
                        for comment in (response.get("comments") or [])[:10]:
                            await store.update_xhs_note_comment(note_id, comment)
                            for child in (comment.get("sub_comments") or [])[:5]:
                                child = dict(child)
                                child["target_comment"] = {"id": comment["id"]}
                                await store.update_xhs_note_comment(note_id, child)
                    except Exception:
                        posts[-1]["comments_error"] = True
                    await asyncio.sleep(2)
            elif args.source == "tieba":
                from media_platform.tieba.core import TieBaCrawler
                crawler = TieBaCrawler()
                crawler.browser_context, crawler.context_page = context, page
                await page.goto(crawler.index_url, wait_until="domcontentloaded", timeout=25000)
                client = await crawler.create_tieba_client(None)
                if not await client.pong(browser_context=context):
                    raise RuntimeError("session")
                batches = (20 + args.limit - 1) // args.limit
                offset = ((args.page - 1) % batches) * args.limit
                notes = await client.get_notes_by_keyword(args.keyword, page=(args.page - 1) // batches + 1, page_size=20)
                search_count = len(notes)
                has_more = offset + args.limit < len(notes) or len(notes) == 20
                for note in notes[offset:offset + args.limit]:
                    data = await client._get_pc_page_data(note.note_id, 1)
                    detail = crawler._page_extractor.extract_note_detail_from_api(data)
                    row = detail.model_dump()
                    row["body"] = tieba_blocks((data.get("first_floor") or {}).get("content"))
                    posts.append(row)
                    raw_comments = {str(i.get("id")): i for i in data.get("post_list", [])}
                    for comment in crawler._page_extractor.extract_tieba_note_parent_comments_from_api(data, detail)[:10]:
                        row = comment.model_dump()
                        row["body"] = tieba_blocks(raw_comments.get(row["comment_id"], {}).get("content"))
                        comments.append(row)
                    await asyncio.sleep(2)
            if search_count and not posts:
                raise RuntimeError("details")
            return {"posts": posts, "comments": comments, "hasMore": has_more, "searchCount": search_count}
        finally:
            await page.close()
        # Playwright disconnects when the context manager ends; user tabs stay open.


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--home", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--source", choices=["xiaohongshu", "tieba"], required=True)
    parser.add_argument("--keyword", required=True)
    parser.add_argument("--page", type=int, default=1)
    parser.add_argument("--limit", type=int, default=3)
    parser.add_argument("--cdp", default="ws://127.0.0.1:9222/devtools/browser")
    args = parser.parse_args()
    args.limit = min(max(args.limit, 1), 5)
    os.chdir(args.home)
    sys.path.insert(0, args.home)
    logging.disable(logging.CRITICAL)
    try:
        result = asyncio.run(asyncio.wait_for(collect(args), timeout=100))
    except Exception as exc:
        # Keep diagnostic type/frame names only: upstream exceptions may contain
        # signed URLs or session data which must never reach logs or the API.
        result = {"error": "session" if str(exc) == "session" else "unavailable", "diagnostic": {"type": type(exc).__name__, "frames": [f.name for f in traceback.extract_tb(exc.__traceback__)]}}
    Path(args.output).write_text(json.dumps(result, ensure_ascii=False), encoding="utf-8")


if __name__ == "__main__":
    main()
