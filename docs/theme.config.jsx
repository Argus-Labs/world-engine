import {useRouter} from "next/router";

export default {
    logo: <span>◢ ✦ ◣</span>,
    project: {
        link: 'https://github.com/argus-labs/world-engine',
    },
    footer: {
        text: (
            <span>
        MIT {new Date().getFullYear()} ©{' '}
                <a href="https://argus.gg" target="_blank">
          ARGUS ◢ ✦ ◣
        </a>
      </span>
        )
    },
    useNextSeoProps() {
        const { asPath } = useRouter()
        return {
            titleTemplate: asPath !== "/" ? "%s | World Engine ◢ ✦ ◣" : "Home | World Engine ◢ ✦ ◣",
            openGraph: {
                images: [{ url: `/img/og.jpg` }],
                siteName: "World Engine",
            },
        };
    },
    head: (
        <>
            <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png" />
            <link rel="icon" type="image/png" sizes="32x32" href="/favicon-32x32.png" />
            <link rel="icon" type="image/png" sizes="16x16" href="/favicon-16x16.png" />
            <link rel="manifest" href="/site.webmanifest" />
            <link rel="mask-icon" href="/safari-pinned-tab.svg" color="#f5d64e" />
            <meta name="msapplication-TileColor" content="#da532c"/ >
            <meta name="theme-color" content="#fffce4" />
        </>
    )
}