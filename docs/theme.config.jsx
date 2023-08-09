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
          Argus Labs ◢ ✦ ◣
        </a>
        .
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
}

