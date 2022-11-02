// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabswitch

import "chromiumos/tast/local/chrome/cuj"

// website defines all web site involved in this test case.
type website string

const (
	// These are the website name of the Google related websites.
	googleCloud      website = "Google Cloud"
	googleFinance    website = "Google Finance"
	googleHelp       website = "Google Help"
	googleNews       website = "Google News"
	googleNonprofits website = "Google Nonprofits"
	googlePlay       website = "Google Play"
	googlePolicy     website = "Google Policy"
	googleStore      website = "Google Store"
	googleWorkspace  website = "Google Workspace"
	youtube          website = "Youtube"

	// These are the website name of the external websites.
	wikipedia    website = "Wikipedia"
	reddit       website = "Reddit"
	medium       website = "Medium"
	yahooNews    website = "YahooNews"
	yahooFinance website = "YahooFinance"
	cnn          website = "CNN"
	espn         website = "ESPN"
	hulu         website = "Hulu"
	pinterest    website = "Pinterest"
	netflix      website = "Netflix"
)

// webPageInfo records a Chrome page's information, including the current browsing page
// and url links (in patterns) for page navigation.
type webPageInfo struct {
	// tier is only used to generate targets.
	tier cuj.Tier
	// webName is the current page's website name.
	webName website
	// contentPatterns holds the patterns of the url links embedded in the web page. During
	// tab switch, we find the url of the given pattern in the current page and click it.
	// Links can be clicked back and forth in case multiple rounds of tab switch are executed.
	contentPatterns []string
}

func newPageInfo(tier cuj.Tier, webname website, patterns ...string) *webPageInfo {
	if len(patterns) < 2 {
		panic("Invalid configuration of webPageInfo")
	}

	return &webPageInfo{
		tier:            tier,
		webName:         webname,
		contentPatterns: patterns,
	}
}

// tabTarget is the struct for tab target used in tab switching test. It contains the URL and the web page info of the tab.
type tabTarget struct {
	url  string
	info *webPageInfo
}

var tabTargetsMap = map[string][]tabTarget{
	"google":   googleWebsitesTargets,
	"external": externalWebsitesTargets,
}

// googleWebsitesTargets defines Google related websites as browse tab targets.
var googleWebsitesTargets = []tabTarget{
	{cuj.GoogleWorkspaceFeaturesURL, newPageInfo(cuj.Essential, googleWorkspace, `/features`, `/products/meet`)},
	{cuj.GoogleWorkspacePricingURL, newPageInfo(cuj.Essential, googleWorkspace, `/pricing`, `/products/calendar`)},
	{cuj.GoogleWorkspaceSecurityURL, newPageInfo(cuj.Essential, googleWorkspace, `/security`, `/products/docs`)},
	{cuj.GoogleWorkspaceFAQURL, newPageInfo(cuj.Advanced, googleWorkspace, `/faq`, `/products/slides`)},
	{cuj.GoogleWorkspaceBusinessURL, newPageInfo(cuj.Advanced, googleWorkspace, `/business`, `/new-business`, `/products/drive`)},
	{cuj.GoogleWorkspaceResourcesURL, newPageInfo(cuj.Advanced, googleWorkspace, `/resources`, `/working-remotely`, `/products/sheets`)},

	{cuj.GoogleStoreURL, newPageInfo(cuj.Essential, googleStore, `/`, `/ideas`, `/cart`)},
	{cuj.GoogleStorePhonesURL, newPageInfo(cuj.Essential, googleStore, `/category/phones`, `/category/earbuds`, `/cart`)},
	{cuj.GoogleStoreOrderHistoryURL, newPageInfo(cuj.Essential, googleStore, `/orderhistory`, `/repairhistory`)},
	{cuj.GoogleStoreSmartHomeURL, newPageInfo(cuj.Advanced, googleStore, `/category/connected_home`, `/product/pixelbook`, `/cart`)},
	{cuj.GoogleStoreInstallationURL, newPageInfo(cuj.Advanced, googleStore, `/magazine/installation`, `/category/watches`, `/cart`)},
	{cuj.GoogleStoreSubscriptionsURL, newPageInfo(cuj.Advanced, googleStore, `/category/subscriptions`, `/support`, `/cart`)},

	{cuj.GoogleHelpChromeURL, newPageInfo(cuj.Essential, googleHelp, `/chrome`, `/community`)},
	{cuj.GoogleHelpYoutubeURL, newPageInfo(cuj.Essential, googleHelp, `/youtube`, `/community`)},
	{cuj.GoogleHelpMailURL, newPageInfo(cuj.Advanced, googleHelp, `/mail`, `/community`)},
	{cuj.GoogleHelpGooglePlayURL, newPageInfo(cuj.Advanced, googleHelp, `/googleplay`, `/community`)},
	{cuj.GoogleHelpMapURL, newPageInfo(cuj.Advanced, googleHelp, `/maps`, `/community`)},

	{cuj.GoogleNonprofitsURL, newPageInfo(cuj.Essential, googleNonprofits, `/`, `/offerings/workspace`, `/resources/faq`)},
	{cuj.GoogleNonprofitsProductHelpURL, newPageInfo(cuj.Essential, googleNonprofits, `/resources/product-help`, `/resources/how-to-guide`)},
	{cuj.GoogleNonprofitsEligibilityURL, newPageInfo(cuj.Advanced, googleNonprofits, `/eligibility`, `/offerings/youtube-nonprofit-program`, `/resources/faq`)},
	{cuj.GoogleNonprofitsSucessStoriesURL, newPageInfo(cuj.Advanced, googleNonprofits, `/success-stories`, `/resources/faq`)},

	{cuj.GooglePlayBooksURL, newPageInfo(cuj.Advanced, googlePlay, `/books`, `/wishlist`, `/FAMILY`)},
	{cuj.GooglePlayKidsURL, newPageInfo(cuj.Advanced, googlePlay, `/apps/category/FAMILY`, `/store/apps/category/FAMILY?age=AGE_RANGE1`, `/games`)},
	{cuj.GooglePlayGameURL, newPageInfo(cuj.Advanced, googlePlay, `/games`, `/store/games?device=tablet`, `/apps`)},
	{cuj.GooglePlayAppsURL, newPageInfo(cuj.Advanced, googlePlay, `/apps`, `/store/apps?device=tablet`, `/movies`)},
	{cuj.GooglePlayMoviesURL, newPageInfo(cuj.Advanced, googlePlay, `/movies`, `/TV`, `/books`)},

	{cuj.GoogleFinanceURL, newPageInfo(cuj.Advanced, googleFinance, `/`, `/markets/indexes`)},
	{cuj.GoogleFinanceIndexesURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/indexes`, `/markets/most-active`, `/markets/gainers`)},
	{cuj.GoogleFinanceMostActiveURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/most-active`, `/markets/gainers`, `/markets/losers`)},
	{cuj.GoogleFinanceGainersURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/gainers`, `/markets/losers`, `/markets/currencies`)},
	{cuj.GoogleFinanceLosersURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/losers`, `/markets/currencies`, `/`)},

	{cuj.GoogleNewsURL, newPageInfo(cuj.Advanced, googleNews, `/home`, `/foryou`, `/my/library`)},
	{cuj.GoogleNewsForYouURL, newPageInfo(cuj.Advanced, googleNews, `/foryou`, `/my/library`, `/`)},

	{cuj.GooglePolicyURL, newPageInfo(cuj.Advanced, googlePolicy, `/`, `privacy`, `terms`)},
	{cuj.GooglePolicyPrivacyURL, newPageInfo(cuj.Advanced, googlePolicy, `privacy`, `faq`, `technologies`)},

	{cuj.YoutubeURL, newPageInfo(cuj.Advanced, youtube, `/`, `/feed/library`, `/history`)},
}

// externalWebsitesTargets defines external websites as browse tab targets.
var externalWebsitesTargets = []tabTarget{
	{cuj.WikipediaMainURL, newPageInfo(cuj.Essential, wikipedia, `/Main_Page`, `/Wikipedia:Contents`)},
	{cuj.WikipediaCurrentEventsURL, newPageInfo(cuj.Essential, wikipedia, `/Portal:Current_events`, `/Special:Random`)},
	{cuj.WikipediaAboutURL, newPageInfo(cuj.Essential, wikipedia, `/Wikipedia:About`, `/Wikipedia:Contact_us`)},
	{cuj.WikipediaHelpURL, newPageInfo(cuj.Advanced, wikipedia, `/Help:Contents`, `/Help:Introduction`)},
	{cuj.WikipediaCommunityURL, newPageInfo(cuj.Advanced, wikipedia, `/Wikipedia:Community_portal`, `/Special:RecentChanges`)},
	{cuj.WikipediaContributionURL, newPageInfo(cuj.Advanced, wikipedia, `/Help:User_contributions`, `/Wikipedia`)},

	{cuj.RedditWallstreetURL, newPageInfo(cuj.Essential, reddit, `/r/wallstreetbets/hot/`, `/r/wallstreetbets/new/`)},
	{cuj.RedditTechNewsURL, newPageInfo(cuj.Essential, reddit, `/r/technews/hot/`, `/r/technews/new/`)},
	{cuj.RedditOlympicsURL, newPageInfo(cuj.Essential, reddit, `/r/olympics/hot/`, `/r/olympics/new/`)},
	{cuj.RedditProgrammingURL, newPageInfo(cuj.Advanced, reddit, `/r/programming/hot/`, `/r/programming/new/`)},
	{cuj.RedditAppleURL, newPageInfo(cuj.Advanced, reddit, `/r/apple/hot/`, `/r/apple/new/`)},
	{cuj.RedditBrooklynURL, newPageInfo(cuj.Advanced, reddit, `/r/brooklynninenine/hot/`, `/r/brooklynninenine/new/`)},

	// Since "Medium" sites change content frequently, add an alternate tag link pattern.
	{cuj.MediumBusinessURL, newPageInfo(cuj.Essential, medium, `/business`, `/economy`, `/money`, `/marketing`)},
	{cuj.MediumStartupURL, newPageInfo(cuj.Essential, medium, `/startup`, `/leadership`, `/marketing`, `/business`)},
	{cuj.MediumWorkURL, newPageInfo(cuj.Advanced, medium, `/work`, `/productivity`, `/careers`, `/business`)},
	{cuj.MediumSoftwareURL, newPageInfo(cuj.Advanced, medium, `/software-engineering`, `/programming`, `/coding`, `/technology`)},
	{cuj.MediumAIURL, newPageInfo(cuj.Advanced, medium, `/artificial-intelligence`, `/data-science`, `/software-engineering`, `/programming`)},

	// Since "Yahoo" sites change content frequently, add an alternate tag link pattern.
	{cuj.YahooUsURL, newPageInfo(cuj.Essential, yahooNews, `/us/`, `/politics/`, `/world/`)},
	{cuj.YahooWorldURL, newPageInfo(cuj.Essential, yahooNews, `/world/`, `/coronavirus/`, `/health/`)},
	{cuj.YahooScienceURL, newPageInfo(cuj.Advanced, yahooNews, `/science/`, `/originals/`, `/us/`)},
	{cuj.YahooFinanaceWatchlistURL, newPageInfo(cuj.Advanced, yahooFinance, `/watchlists/`, `/news/`)},

	{cuj.CnnWorldURL, newPageInfo(cuj.Advanced, cnn, `/world`, `/africa`)},
	{cuj.CnnAmericasURL, newPageInfo(cuj.Advanced, cnn, `/americas`, `/asia`)},
	{cuj.CnnAustraliaURL, newPageInfo(cuj.Advanced, cnn, `/australia`, `/china`)},
	{cuj.CnnEuropeURL, newPageInfo(cuj.Advanced, cnn, `/europe`, `/india`)},
	{cuj.CnnMiddleEastURL, newPageInfo(cuj.Advanced, cnn, `/middle-east`, `/uk`)},

	{cuj.EspnNflURL, newPageInfo(cuj.Advanced, espn, `/nfl/scoreboard`, `/nfl/schedule`)},
	{cuj.EspnNbaURL, newPageInfo(cuj.Advanced, espn, `/nba/scoreboard`, `/nba/schedule`)},
	{cuj.EspnCollegeBasketballURL, newPageInfo(cuj.Advanced, espn, `/mens-college-basketball/scoreboard`, `/mens-college-basketball/schedule`)},
	{cuj.EspnTennisURL, newPageInfo(cuj.Advanced, espn, `/tennis/dailyResults`, `/tennis/schedule`)},
	{cuj.EspnSoccerURL, newPageInfo(cuj.Advanced, espn, `/soccer/scoreboard`, `/soccer/schedule`)},

	{cuj.HuluMoviesURL, newPageInfo(cuj.Advanced, hulu, `/hub/movies`, `/hub/originals`)},
	{cuj.HuluKidsURL, newPageInfo(cuj.Advanced, hulu, `/hub/kids`, `/hub/networks`)},

	{cuj.PinterestURL, newPageInfo(cuj.Advanced, pinterest, `/ideas/`, `/ideas/holidays/910319220330/`)},

	{cuj.NetflixURL, newPageInfo(cuj.Advanced, netflix, `/en`, `/en/legal/termsofuse`)},

	{cuj.YoutubeURL, newPageInfo(cuj.Advanced, youtube, `/`, `/feed/explore`)},
}
