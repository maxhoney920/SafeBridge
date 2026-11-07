// Готовые наборы сайтов для списков «Через VPN» и «Без VPN».
// Из этого файла собирается встроенная копия и lists.json в ветке downloads: оттуда
// расширение и приложение берут новые наборы без обновления (раз в 6 часов и по кнопке).
// Меняете наборы — увеличьте rev и поставьте дату в updated: по rev клиенты понимают,
// что вышла новая версия списков.
// Поля набора: name — название; domains — адреса; main — основные адреса (по ним
// набор узнаётся на странице и в списке пользователя; по умолчанию первые два адреса);
// service — набор одного сервиса (только такие предлагаются в «Без VPN» со страницы).
export const LISTS = {
  rev: 2,
  updated: '2026-11-08',
  vpn: [
    { name: 'YouTube', domains: ['youtube.com', 'youtu.be', 'googlevideo.com', 'ytimg.com', 'ggpht.com', 'youtube-nocookie.com', 'youtubei.googleapis.com', 'youtube.googleapis.com', 'jnn-pa.googleapis.com', 'youtubekids.com', 'yt.be'] },
    { name: 'Instagram', domains: ['instagram.com', 'cdninstagram.com', 'fbcdn.net', 'ig.me'] },
    { name: 'Facebook', domains: ['facebook.com', 'fb.com', 'fbcdn.net', 'messenger.com', 'facebook.net', 'fbsbx.com'] },
    { name: 'X (Twitter)', domains: ['x.com', 'twitter.com', 'twimg.com', 't.co'] },
    { name: 'Discord', domains: ['discord.com', 'discord.gg', 'discordapp.com', 'discordapp.net', 'discord.media', 'discordcdn.com'] },
    { name: 'ChatGPT', domains: ['chatgpt.com', 'openai.com', 'oaistatic.com', 'oaiusercontent.com', 'sora.com'] },
    { name: 'Claude', domains: ['claude.ai', 'anthropic.com', 'claudeusercontent.com', 'claude.com'] },
    // Gemini, AI Studio, NotebookLM и другие сервисы Google AI. Страница открывается с gemini.google.com,
    // а сами ответы идут через служебные адреса *-pa.googleapis.com и *.clients6.google.com:
    // без них Google видит настоящий адрес и пишет, что сервис в вашей стране недоступен.
    // Список сверен с v2fly/domain-list-community (google-deepmind) и itdoginfo/allow-domains.
    { name: 'Gemini', main: ['gemini.google.com', 'aistudio.google.com', 'gemini.google', 'notebooklm.google.com', 'notebooklm.google', 'labs.google', 'ai.google.dev'],
      domains: ['gemini.google.com', 'aistudio.google.com', 'gemini.google', 'bard.google.com', 'gemini.gstatic.com',
        'generativelanguage.googleapis.com', 'robinfrontend-pa.googleapis.com', 'proactivebackend-pa.googleapis.com',
        'geller-pa.googleapis.com', 'aisandbox-pa.googleapis.com', 'aida.googleapis.com', 'speechs3proto2-pa.googleapis.com',
        'clients6.google.com', 'makersuite.google.com', 'ai.google.dev', 'ai.studio', 'generativeai.google',
        'notebooklm.google.com', 'notebooklm.google', 'notebooklm-pa.googleapis.com', 'notebooklm.googleapis.com',
        'labs.google', 'labs.google.com', 'jules.google', 'jules.google.com', 'deepmind.google', 'deepmind.com'] },
    { name: 'TikTok', domains: ['tiktok.com', 'tiktokcdn.com', 'tiktokv.com', 'ttwstatic.com', 'byteoversea.com', 'ibytedtos.com', 'tiktokcdn-us.com', 'tiktokcdn-eu.com'] },
    { name: 'Twitch', domains: ['twitch.tv', 'ttvnw.net', 'jtvnw.net', 'twitchcdn.net', 'twitchsvc.net', 'ext-twitch.tv'] },
    { name: 'Spotify', domains: ['spotify.com', 'scdn.co', 'spotifycdn.com', 'spotilocal.com', 'pscdn.co'] },
    { name: 'Netflix', domains: ['netflix.com', 'nflxvideo.net', 'nflximg.net', 'nflxext.com', 'nflxso.net'] },
    { name: 'LinkedIn', domains: ['linkedin.com', 'licdn.com'] },
    { name: 'Reddit', domains: ['reddit.com', 'redd.it', 'redditmedia.com', 'redditstatic.com'] },
    { name: 'Medium', domains: ['medium.com', 'miro.medium.com'] },
    { name: 'Telegram Web', domains: ['telegram.org', 't.me', 'telegram.me', 'telesco.pe'] },
    // Торрент-трекеры. Адреса сверены с v2fly/domain-list-community (rutracker) и
    // itdoginfo/allow-domains (Russia/inside). t-ru.org — анонсер RuTracker (bt*.t-ru.org):
    // через него торрент-клиент узнаёт о раздающих.
    { name: 'RuTracker', main: ['rutracker.org', 'rutracker.net', 'rutracker.cc'],
      domains: ['rutracker.org', 'rutracker.net', 'rutracker.cc', 'rutracker.cr', 'rutracker.me', 'rutracker.nl',
        'rutracker.wiki', 'rutrk.org', 'maintracker.org', 't-ru.org'] },
    { name: 'LostFilm', main: ['lostfilm.tv', 'lostfilm.download', 'lostfilm.uno', 'lostfilm.tw'],
      domains: ['lostfilm.tv', 'lostfilm.download', 'lostfilm.uno', 'lostfilm.tw', 'lostfilm.top', 'lostfilmtv2.site', 'insearch.site'] },
    { name: 'NNM-Club', main: ['nnmclub.to', 'nnm-club.me'], domains: ['nnmclub.to', 'nnm-club.me', 'nnmstatic.win'] },
    { name: 'Rutor', main: ['rutor.info', 'rutor.is', 'rutor.org'], domains: ['rutor.info', 'rutor.is', 'rutor.org'] },
    { name: 'Kinozal', main: ['kinozal.tv', 'kinozal.me', 'kinozal.guru'], domains: ['kinozal.tv', 'kinozal.me', 'kinozal.guru', 'kinozaltv.life'] },
    { name: 'AniLibria', main: ['anilibria.tv', 'anilibria.top', 'anilibria.org'], domains: ['anilibria.tv', 'anilibria.org', 'anilibria.uno', 'anilibria.wtf', 'anilibria.top', 'anilib.me'] },
    { name: 'Другие трекеры', main: ['rustorka.com', 'megapeer.vip', 'riperam.org', 'torrentgalaxy.to'],
      domains: ['rustorka.com', 'megapeer.ru', 'megapeer.vip', 'riperam.org', 'torrentgalaxy.to', 'fast-torrent.club', 'filmitorrent.net', 'cloudtorrents.com'] },
  ],
  direct: [
    { name: 'Все сайты .ru, .рф, .su', domains: ['ru', 'xn--p1ai', 'su'] },
    { name: 'Госуслуги и налоги', domains: ['gosuslugi.ru', 'nalog.gov.ru', 'mos.ru', 'esia.gosuslugi.ru'] },
    { name: 'Банки', domains: ['sberbank.ru', 'tbank.ru', 'tinkoff.ru', 'vtb.ru', 'alfabank.ru', 'gazprombank.ru', 'raiffeisen.ru'] },
    { name: 'Маркетплейсы', domains: ['ozon.ru', 'wildberries.ru', 'wb.ru', 'avito.ru', 'market.yandex.ru'] },
    { name: 'Яндекс', service: true, domains: ['yandex.ru', 'ya.ru', 'yandex.net', 'yastatic.net', 'yandex.com'] },
    { name: 'VK и Mail.ru', service: true, domains: ['vk.com', 'vk.ru', 'userapi.com', 'vkuser.net', 'mail.ru', 'ok.ru'] },
  ],
};
