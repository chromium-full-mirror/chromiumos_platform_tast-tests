// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabswitch

import (
	"context"
	"fmt"
	"os"
	"path"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/cuj"
)

// website defines all web site involved in this test case.
type website string

const (
	// These are the website name of the Google related websites.
	googleFinance    website = "Google Finance"
	googleHelp       website = "Google Help"
	googleNonprofits website = "Google Nonprofits"
	googlePlay       website = "Google Play"
	googlePolicy     website = "Google Policy"
	googleStore      website = "Google Store"
	googleWorkspace  website = "Google Workspace"

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
	youtube      website = "Youtube"

	// These are the files' and folder's name of the local websites.
	localWebsite website = "localWebsite"
	// These files are stored in the LocalWebZIPFile and will be duplicated in the generateLocalWebsitesTargets function.
	localGIF    = "TabSwitchLocalGIF"
	localGraph  = "TabSwitchLocalGraph"
	localScript = "TabSwitchLocalScript"
	localVideo  = "TabSwitchLocalVideo"
	// These HTML files will be generated in the generateLocalWebsitesTargets function.
	localHyperlink = "TabSwitchLocalHyperLink"
	localHTML      = "TabSwitchLocalWebsite"

	// Generate multiple HTML elements of each kind: script (file), script (inline), image (static), and image (gif) for each local HTML file.
	maxDuplicateElements    = 15
	mediumDuplicateElements = 10
	minDuplicateElements    = 5

	// localText is an introduction extracted from the wikipedia "Chromebook" page. The text will be shown in the local web pages.
	localText = `
	<p>A <b>Chromebook</b> (sometimes stylized in lowercase as <b>chromebook</b>) is a laptop or tablet running the Linux-based ChromeOS as its operating system. Initially designed to heavily rely on web applications for tasks using the Google Chrome browser, Chromebooks have since expanded to be able to run Android and full-fledged Linux apps since 2017 and 2018, respectively. All supported apps can be installed and launched alongside each other.</p>
	<p>Chromebooks can work offline; applications like Gmail, Google Calendar, Google Keep, and Google Drive synchronize data when reconnecting to the Internet. Google Play video content is available offline using the Google Play Movies &amp; TV extension with the Chrome browser.</p>
	<p>The first Chromebooks shipped on June 15, 2011. Other form factors include Chromebox desktops, Chromebase, which places the computer in an all-in-one unit, an HDMI stick PC called a Chromebit, and Chromebook tablets.</p>
	<p>In 2020, Chromebooks outsold Apple Macs for the first time by taking market share from laptops running Microsoft Windows. This rise is attributed to the platform's success in the education market.</p>
	`
)

var (
	windowNumberMap = map[cuj.Tier]int{
		cuj.Essential: 2,
		cuj.Advanced:  4,
	}
	tabNumberMap = map[cuj.Tier]int{
		cuj.Essential: 5,
		cuj.Advanced:  9,
	}
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

func newPageInfo(tier cuj.Tier, webName website, patterns ...string) *webPageInfo {
	if len(patterns) < 2 {
		panic("Invalid configuration of webPageInfo")
	}

	return &webPageInfo{
		tier:            tier,
		webName:         webName,
		contentPatterns: patterns,
	}
}

// tabTarget is the struct for tab target used in tab switching test. It contains the URL and the web page info of the tab.
type tabTarget struct {
	url  string
	info *webPageInfo
}

// tabTargetsMap maps the web source type to the tab targets.
var tabTargetsMap = map[cuj.WebSourceType][]tabTarget{
	cuj.GoogleWebSource:   googleWebsitesTargets,
	cuj.ExternalWebSource: externalWebsitesTargets,
	// Keep the key "localWebSource" in the map to help verify if the given web source is supported.
	cuj.LocalWebSource: {},
}

// googleWebsitesTargets defines Google related websites as browse tab targets.
var googleWebsitesTargets = []tabTarget{
	{cuj.GoogleWorkspaceFeaturesURL, newPageInfo(cuj.Essential, googleWorkspace, `/features`, `/products/meet`)},
	{cuj.GoogleWorkspacePricingURL, newPageInfo(cuj.Essential, googleWorkspace, `/pricing`, `/products/calendar`)},
	{cuj.GoogleWorkspaceSecurityURL, newPageInfo(cuj.Essential, googleWorkspace, `/security`, `/products/docs`)},
	{cuj.GoogleWorkspaceFAQURL, newPageInfo(cuj.Advanced, googleWorkspace, `/faq`, `/products/slides`)},
	{cuj.GoogleWorkspaceBusinessURL, newPageInfo(cuj.Advanced, googleWorkspace, `/business`, `/new-business`, `/products/drive`)},
	{cuj.GoogleWorkspaceResourcesURL, newPageInfo(cuj.Advanced, googleWorkspace, `/resources`, `/working-remotely`, `/products/sheets`)},
	{cuj.GoogleWorkspaceGmailURL, newPageInfo(cuj.Advanced, googleWorkspace, `/products/gmail`, `/products/chat`, `/products/admin`)},

	{cuj.GoogleStoreURL, newPageInfo(cuj.Essential, googleStore, `/`, `/ideas`, `/cart`)},
	{cuj.GoogleStorePhonesURL, newPageInfo(cuj.Essential, googleStore, `/category/phones`, `/category/earbuds`, `/cart`)},
	{cuj.GoogleStoreRepairCenterURL, newPageInfo(cuj.Essential, googleStore, `/magazine/repaircenter`, `/repair`, `/cart`)},
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
	{cuj.GoogleNonprofitsSuccessStoriesURL, newPageInfo(cuj.Advanced, googleNonprofits, `/success-stories`, `/resources/faq`)},
	{cuj.GoogleNonprofitsWorkspaceURL, newPageInfo(cuj.Advanced, googleNonprofits, `/offerings/workspace/`, `/offerings/google-ad-grants`, `/resources/faq`)},

	{cuj.GooglePlayBooksURL, newPageInfo(cuj.Advanced, googlePlay, `/books`, `/wishlist`, `/FAMILY`)},
	{cuj.GooglePlayKidsURL, newPageInfo(cuj.Advanced, googlePlay, `/apps/category/FAMILY`, `/store/apps/category/FAMILY?age=AGE_RANGE1`, `/games`)},
	{cuj.GooglePlayGameURL, newPageInfo(cuj.Advanced, googlePlay, `/games`, `/store/games?device=tablet`, `/apps`)},
	{cuj.GooglePlayAppsURL, newPageInfo(cuj.Advanced, googlePlay, `/apps`, `/store/apps?device=tablet`, `/movies`)},
	{cuj.GooglePlayMoviesURL, newPageInfo(cuj.Advanced, googlePlay, `/movies`, `/TV`, `/books`)},

	{cuj.GoogleFinanceURL, newPageInfo(cuj.Advanced, googleFinance, `/`, `/markets/indexes`)},
	{cuj.GoogleFinanceIndexesURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/indexes`, `/markets/most-active`, `/markets/gainers`)},
	{cuj.GoogleFinanceMostActiveURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/most-active`, `/markets/currencies`, `/markets/losers`)},
	{cuj.GoogleFinanceGainersURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/gainers`, `/markets/losers`, `/markets/currencies`)},
	{cuj.GoogleFinanceLosersURL, newPageInfo(cuj.Advanced, googleFinance, `/markets/losers`, `/markets/gainers`, `/`)},

	{cuj.GooglePolicyURL, newPageInfo(cuj.Advanced, googlePolicy, `/`, `privacy`, `terms`)},
	{cuj.GooglePolicyPrivacyURL, newPageInfo(cuj.Advanced, googlePolicy, `privacy`, `faq`, `technologies`)},
	{cuj.GooglePolicyFAQURL, newPageInfo(cuj.Advanced, googlePolicy, `faq`, `technologies`, `terms`)},
	{cuj.GooglePolicyTechnologiesURL, newPageInfo(cuj.Advanced, googlePolicy, `technologies`, `terms`, `faq`)},
}

// externalWebsitesTargets defines external websites as browse tab targets.
var externalWebsitesTargets = []tabTarget{
	{cuj.WikipediaMainURL, newPageInfo(cuj.Essential, wikipedia, `/Main_Page`, `/Wikipedia:Contents`)},
	{cuj.WikipediaCurrentEventsURL, newPageInfo(cuj.Essential, wikipedia, `/Portal:Current_events`, `/Special:Random`)},
	{cuj.WikipediaAboutURL, newPageInfo(cuj.Essential, wikipedia, `/Wikipedia:About`, `/Wikipedia:Contact_us`)},
	{cuj.WikipediaHelpURL, newPageInfo(cuj.Advanced, wikipedia, `/Help:Contents`, `/Help:Introduction`)},
	{cuj.WikipediaCommunityURL, newPageInfo(cuj.Advanced, wikipedia, `/Wikipedia:Community_portal`, `/Special:RecentChanges`)},
	{cuj.WikipediaContributionURL, newPageInfo(cuj.Advanced, wikipedia, `/Help:User_contributions`, `/Wikipedia`)},

	{cuj.RedditWallStreetURL, newPageInfo(cuj.Essential, reddit, `/r/wallstreetbets/hot/`, `/r/wallstreetbets/new/`)},
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
	{cuj.YahooFinanceWatchListURL, newPageInfo(cuj.Advanced, yahooFinance, `/watchlists/`, `/news/`)},

	{cuj.CnnWorldURL, newPageInfo(cuj.Advanced, cnn, `/world`, `/africa`)},
	{cuj.CnnAmericasURL, newPageInfo(cuj.Advanced, cnn, `/americas`, `/asia`)},
	{cuj.CnnAustraliaURL, newPageInfo(cuj.Advanced, cnn, `/australia`, `/china`)},
	{cuj.CnnEuropeURL, newPageInfo(cuj.Advanced, cnn, `/europe`, `/india`)},
	{cuj.CnnMiddleEastURL, newPageInfo(cuj.Advanced, cnn, `/middle-east`, `/europe`, `/united-kingdom`)},

	{cuj.EspnNflURL, newPageInfo(cuj.Advanced, espn, `/nfl/scoreboard`, `/nfl/schedule`)},
	{cuj.EspnNbaURL, newPageInfo(cuj.Advanced, espn, `/nba/scoreboard`, `/nba/schedule`)},
	{cuj.EspnCollegeBasketballURL, newPageInfo(cuj.Advanced, espn, `/mens-college-basketball/scoreboard`, `/mens-college-basketball/schedule`)},
	{cuj.EspnTennisURL, newPageInfo(cuj.Advanced, espn, `/tennis/dailyResults`, `/tennis/schedule`)},
	{cuj.EspnSoccerURL, newPageInfo(cuj.Advanced, espn, `/soccer/scoreboard`, `/soccer/schedule`)},

	{cuj.HuluMoviesURL, newPageInfo(cuj.Advanced, hulu, `/hub/movies`, `/hub/originals`)},
	{cuj.HuluKidsURL, newPageInfo(cuj.Advanced, hulu, `/hub/kids`, `/hub/networks`)},

	{cuj.PinterestURL, newPageInfo(cuj.Advanced, pinterest, `/ideas/`, `/ideas/holidays/910319220330/`)},

	{cuj.NetflixURL, newPageInfo(cuj.Advanced, netflix, `/en`, `/en/legal/termsofuse`)},

	{cuj.YoutubeURL, newPageInfo(cuj.Advanced, youtube, `/`, `/feed/library`, `/history`)},
}

// generateLocalWebsitesTargets generates a list of local HTML files.
func generateLocalWebsitesTargets(ctx context.Context, dirPath, localURL string, tier cuj.Tier) error {
	winNum, ok1 := windowNumberMap[tier]
	tabNum, ok2 := tabNumberMap[tier]
	if !ok1 || !ok2 {
		return errors.Errorf("unacceptable tier: %v", tier)
	}

	hyperlink := fmt.Sprintf(`<a href="./%s.html">test</a>`, localHyperlink)

	inputTextBox := `
		<label for="input text">Input your text:</label><br>
		<input type="text" id="input text" name="input text"><br>
	`

	hideTextButtonAndScript := `
		<button id="text button" onclick="hideText()">Hide text</button>
		<script>
		function hideText() {
			let text = document.getElementById("text");
			let button = document.getElementById("text button");
			if (text.style.display === "none") {
				text.style.display = "block";
				button.textContent = "Hide text";
			} else {
				text.style.display = "none";
				button.textContent = "Show text";
			}
		}
		</script>
	`

	videoPath := path.Join(dirPath, localVideo+".mp4")
	scriptPath := path.Join(dirPath, localScript+".js")
	graphPath := path.Join(dirPath, localGraph+".jpg")
	var gifPaths []string
	for i := 0; i < maxDuplicateElements; i++ {
		gifFileName := fmt.Sprintf("%s%d.gif", localGIF, i)
		gifPath := path.Join(dirPath, gifFileName)
		gifPaths = append(gifPaths, gifPath)
	}
	var localWebsitesTargets []tabTarget
	for i := 0; i < winNum*tabNum+1; i++ {
		targetVideoFileName := fmt.Sprintf("%s%d.mp4", localVideo, i)
		targetVideoFilePath := path.Join(dirPath, targetVideoFileName)
		if err := forceSymlink(ctx, videoPath, targetVideoFilePath); err != nil {
			return errors.Wrapf(err, "failed to create symbolic link %s", targetVideoFileName)
		}

		var video, externalScript, inlineScript, smallImageDiv, gifImageDiv string
		// Add video once per two pages to reduce the overall complexity of the local websites.
		if i%2 == 1 {
			var canvas string
			// Render the video in canvas with the drawImage API per four pages to simulate the behavior of the Google properties.
			if i%4 == 1 {
				canvas = `
				<div class="d1">
					<canvas id="canvas"></canvas>
					<script>
						const video = document.getElementById("video");
						const canvas = document.getElementById("canvas");
						const ctx = canvas.getContext("2d");

						document.addEventListener("visibilitychange", function() {
							if (!document.hidden){
								video.play();
							}
						});

						video.addEventListener("play", () => {
							function drawVideo() {
								ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
								requestAnimationFrame(drawVideo);
							}
							requestAnimationFrame(drawVideo);
						});
						video.style.visibility = "hidden";
					</script>
				</div>
			`
			}
			video = fmt.Sprintf(`
			<video id="video" width="200" height="150" autoplay muted loop>
				<source src="./%s" type="video/mp4">
			</video>
			%s
			`, targetVideoFileName, canvas)
		}

		duplicateElements := minDuplicateElements
		if i%3 == 1 {
			duplicateElements = mediumDuplicateElements
		} else if i%3 == 2 {
			duplicateElements = maxDuplicateElements
		}

		for j := 0; j < duplicateElements; j++ {
			targetScriptFileName := fmt.Sprintf("%s%d.%d.js", localScript, i, j)
			targetScriptFilePath := path.Join(dirPath, targetScriptFileName)
			if err := forceSymlink(ctx, scriptPath, targetScriptFilePath); err != nil {
				return errors.Wrapf(err, "failed to create symbolic link %s", targetScriptFileName)
			}
			externalScript += fmt.Sprintf(`<script type="text/javascript" async="" src="./%s"></script>`, targetScriptFileName)

			inlineScript += fmt.Sprint(`<script type="text/javascript">
				for (let i = 1; i < 6666666; i++) {
					var j = Math.sqrt(i)}
				</script>`)

			targetGraphFileName := fmt.Sprintf("%s%d.%d.jpg", localGraph, i, j)
			targetGraphFilePath := path.Join(dirPath, targetGraphFileName)
			if err := forceSymlink(ctx, graphPath, targetGraphFilePath); err != nil {
				return errors.Wrapf(err, "failed to create symbolic link %s", targetGraphFileName)
			}
			smallImageDiv += fmt.Sprintf(`<div class="row"><img src="./%s" > </div>`, targetGraphFileName)

			targetGIFFileName := fmt.Sprintf("%s%d.%d.gif", localGIF, i, j)
			// Each element in gifContents will be used one time in each HTML file.
			targetGIFFilePath := path.Join(dirPath, targetGIFFileName)
			if err := forceSymlink(ctx, gifPaths[j], targetGIFFilePath); err != nil {
				return errors.Wrapf(err, "failed to create symbolic link %s", targetGIFFileName)
			}
			gifImageDiv += fmt.Sprintf(`<div class="row"><img src="./%s" style="width:100%%; height:100%%" ></div>`, targetGIFFileName)
		}
		htmlContent := fmt.Sprintf(`<!DOCTYPE html>
	<html>
	<style>
		.float-container {
			display: flex;
		}
		.float-child {
			width: 25%%;
		}
		.table{
			display: grid;
		}
		.row {
		display: flex;
		height: 10vh;
		}
		.column {
		flex: 20%%;
		}

		@keyframes move {
			0%%   {left:0%%; top:0%%;}
			50%%  {left:0%%; top:50%%;}
			100%% {left:0%%; top:0%%;}
		}

		.d1 {
			z-index: 1;
			left: 0%%;
			top: 0%%;
			position: relative;
			animation-name: move;
			animation-duration: 6s;
			animation-iteration-count: infinite;
			animation-delay: 1s;
	}
	</style>
	<head>
		%s
	</head>
	<body>
		%s
		<div class="float-container">
			<div class="float-child">
				%s
			</div>
				%s
			<div class="float-child" >
				%s
				%s
			</div>
			<div class="float-child">
				%s
			</div>
			<div class="float-child">
				%s
				<div id="text">
					%s
				</div>
			</div>
		</div>

		<p>Hyperlinks</p>
		%s
	</body>
	</html>`, externalScript, inlineScript, smallImageDiv, inlineScript, inputTextBox, video, gifImageDiv, hideTextButtonAndScript, localText, hyperlink)
		var htmlFileName string
		if i == 0 {
			htmlFileName = fmt.Sprintf("%s.html", localHyperlink)
		} else {
			htmlFileName = fmt.Sprintf("%s%d.html", localHTML, i)
			// Create URL with fmt.Sprintf function since the path.Join function will convert the "https://"" into "http:/" and cause error when reconnecting to the page.
			htmlURL := fmt.Sprintf("%s/%s", localURL, htmlFileName)
			localWebsiteTarget := tabTarget{htmlURL, newPageInfo(tier, localWebsite, `/`, fmt.Sprintf(`/%s.html`, localHyperlink))}
			localWebsitesTargets = append(localWebsitesTargets, localWebsiteTarget)
		}
		if err := os.WriteFile(path.Join(dirPath, htmlFileName), []byte(htmlContent), 0644); err != nil {
			return errors.Wrapf(err, "failed to write HTML file %s", htmlFileName)
		}
	}
	tabTargetsMap[cuj.LocalWebSource] = localWebsitesTargets
	return nil
}

// forceSymlink creates the symbolic link and force overwrite if the link already exists.
// The overwrite flag is needed to avoid the errors caused by the existing files.
func forceSymlink(ctx context.Context, filePath, linkPath string) error {
	if err := testexec.CommandContext(ctx, "ln", "-sf", filePath, linkPath).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create the symbolic link with the force overwrite option")
	}
	return nil
}
