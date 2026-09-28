-- Browser detection macro using User-Agent string.
-- Detection order is critical:
--   1. In-app / social browsers (they embed Chrome/Safari WebViews)
--   2. Specialized Chromium wrappers (contain "Chrome" in UA)
--   3. Named Chromium forks: Edge, Opera, Brave, Vivaldi, Samsung, Yandex, Electron
--   4. Chrome WebView (before Chrome — has "wv)" marker)
--   5. Mobile Chrome vs Chrome (both have "Chrome/")
--   6. Mobile Firefox vs Firefox
--   7. Mobile Safari vs Safari (all Chromium UAs also contain "Safari/")
--   8. Legacy: IE, Android Browser, generic WebKit
CREATE OR REPLACE MACRO hk_browser(ua) AS
    CASE
        WHEN ua IS NULL OR TRIM(ua) = '' THEN '(Unknown)'
        -- 1. In-app / social browsers
        WHEN ua ILIKE '%FBAN/%' OR ua ILIKE '%FBAV/%' THEN 'Facebook'
        WHEN ua ILIKE '%Instagram%' THEN 'Instagram'
        WHEN ua ILIKE '%musical_ly%' OR ua ILIKE '%TikTok%' OR ua ILIKE '%BytedanceWebview%' THEN 'TikTok'
        WHEN ua ILIKE '%Twitter%' THEN 'Twitter'
        WHEN ua ILIKE '%LinkedInApp%' THEN 'LinkedIn'
        WHEN ua ILIKE '%MicroMessenger%' THEN 'WeChat'
        WHEN ua ILIKE '% Line/%' THEN 'Line'
        -- 2. Specialized Chromium wrappers (must precede Chrome)
        WHEN ua ILIKE '%GSA/%' THEN 'GSA'
        WHEN ua ILIKE '%DuckDuckGo/%' THEN 'DuckDuckGo'
        WHEN ua ILIKE '%Ecosia%' THEN 'Ecosia'
        WHEN ua ILIKE '%Whale/%' THEN 'Whale'
        WHEN ua ILIKE '%QQBrowser/%' THEN 'QQBrowser'
        WHEN ua ILIKE '%Quark/%' THEN 'Quark'
        WHEN ua ILIKE '%UCBrowser/%' OR ua ILIKE '%UBrowser/%' THEN 'UCBrowser'
        WHEN ua ILIKE '%MiuiBrowser/%' THEN 'MIUI Browser'
        WHEN ua ILIKE '%HuaweiBrowser/%' THEN 'Huawei Browser'
        WHEN ua ILIKE '%VivoBrowser/%' THEN 'Vivo Browser'
        WHEN ua ILIKE '%360SE/%' OR ua ILIKE '%360Browser/%' OR ua ILIKE '%QIHU%' THEN '360'
        WHEN ua ILIKE '%Avast/%' OR ua ILIKE '%AvastSecureBrowser%' THEN 'Avast Secure Browser'
        WHEN ua ILIKE '%AVG/%' OR ua ILIKE '%AVGSecureBrowser%' THEN 'AVG Secure Browser'
        WHEN ua ILIKE '%Silk/%' THEN 'Silk'
        -- 3. Named Chromium forks
        WHEN ua ILIKE '%Edg/%' OR ua ILIKE '%EdgA/%' OR ua ILIKE '%EdgiOS/%' THEN 'Edge'
        WHEN ua ILIKE '%OPT/%' THEN 'Opera Touch'
        WHEN ua ILIKE '%OPGX/%' THEN 'Opera GX'
        WHEN ua ILIKE '%OPR/%' OR ua ILIKE '%Opera%' THEN 'Opera'
        WHEN ua ILIKE '%Brave%' THEN 'Brave'
        WHEN ua ILIKE '%Vivaldi%' THEN 'Vivaldi'
        WHEN ua ILIKE '%SamsungBrowser%' THEN 'Samsung Internet'
        WHEN ua ILIKE '%YaBrowser/%' THEN 'Yandex'
        WHEN ua ILIKE '%Electron/%' THEN 'Electron'
        -- 4. Chrome WebView (has "; wv)" before the Chrome token)
        WHEN ua ILIKE '%; wv)%' AND (ua ILIKE '%Chrome/%' OR ua ILIKE '%CriOS/%') THEN 'Chrome WebView'
        -- 5. Firefox (mobile vs desktop)
        WHEN (ua ILIKE '%Firefox/%' OR ua ILIKE '%FxiOS/%') AND ua ILIKE '%Mobile%' THEN 'Mobile Firefox'
        WHEN ua ILIKE '%Firefox/%' OR ua ILIKE '%FxiOS/%' THEN 'Firefox'
        -- 6. Chrome (mobile vs desktop)
        WHEN (ua ILIKE '%Chrome/%' OR ua ILIKE '%CriOS/%') AND ua ILIKE '%Mobile%' THEN 'Mobile Chrome'
        WHEN ua ILIKE '%Chrome/%' OR ua ILIKE '%CriOS/%' THEN 'Chrome'
        -- 7. Safari (mobile vs desktop — must be after Chrome since Chromium UAs contain "Safari/")
        WHEN ua ILIKE '%Safari/%' AND ua ILIKE '%Mobile%' THEN 'Mobile Safari'
        WHEN ua ILIKE '%Safari/%' THEN 'Safari'
        -- 8. Legacy
        WHEN ua ILIKE '%Trident%' OR ua ILIKE '%MSIE%' THEN 'IE'
        WHEN ua ILIKE '%Android%' THEN 'Android Browser'
        WHEN ua ILIKE '%WebKit%' THEN 'WebKit'
        ELSE '(Other)'
    END;
