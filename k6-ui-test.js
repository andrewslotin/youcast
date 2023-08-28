import { check, fail } from 'k6'
import { browser } from 'k6/experimental/browser'

export const options = {
    scenarios: {
        browser: {
            executor: 'shared-iterations',
            options: {
                browser: { type: 'chromium' },
            },
        },
    },
}

export default async function() {
    const page = browser.newPage();

    try {
        await page.goto('http://localhost:8080/');

        check(page, {
            bookmarkletButton: page.locator("#bookmarklet-btn").isVisible(),
            subscribeButton: page.locator("#subscribe-btn").isVisible(),
            sourceTabs: page.locator("#source-tabs").isVisible(),
        });

        const ytTab = page.locator("[href='#add-youtube-video']");

        if (!check(ytTab, {
            tabVisible: ytTab.isVisible(),
        }, {
            tab: "YouTube",
        })) {
            fail("YouTube tab is not visible");
        }

        await ytTab.click();
        check(page, {
            youtubeVideoURL: page.locator("input[name='url']").isVisible(),
            youtubeVideoButton: page.locator("#add-youtube-video-btn").isVisible(),
            uploadFile: page.locator("input[name='media']").isHidden(),
        }, {
            tab: "YouTube",
        });

        const mediaTab = page.locator("[href='#upload-file']");
        if (!check(mediaTab, {
            tabVisible: mediaTab.isVisible(),
        }, {
            tab: "User media",
        })) {
            fail("User media tab is not visible");
        }

        await mediaTab.click();
        check(page, {
            youtubeVideoURL: page.locator("input[name='url']").isHidden(),
            youtubeVideoButton: page.locator("#add-youtube-video-btn").isHidden(),
            uploadFile: page.locator("input[name='media']").isVisible(),
        }, {
            tab: "User media",
        });
    } finally {
        await page.close();
    }
}
