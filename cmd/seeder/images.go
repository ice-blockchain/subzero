// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"

	imgbb "github.com/JohnNON/ImgBB"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/pkg/errors"
)

var images []string = []string{
	"https://i.ibb.co/jJYm4Nz/rena2019.webp",
	"https://i.ibb.co/dcCfp6j/mantanweb-rss-bot.webp",
	"https://i.ibb.co/sFbNpww/China-s-Reevaluation-of-Taiwan-Seizure-Plans-After-Iran-s-Failed-Attack-on-Israel.webp",
	"https://i.ibb.co/k60YSCN/dfs.webp",
	"https://i.ibb.co/thS82gL/Discovering-America-s-Hidden-Gems-Where-to-Mine-Diamonds-and-Gemstones.webp",
	"https://i.ibb.co/5hJcD1g/ASML-s-Disappointing-Results-Cause-Tech-Stocks-to-Slump-in-Europe.webp",
	"https://i.ibb.co/F7xk7ks/Solliday-Teichmann.webp",
	"https://i.ibb.co/ZJ0NJTM/Lady-Gaga.webp",
	"https://i.ibb.co/9yNxF37/Tamarra-Maillet.webp",
	"https://i.ibb.co/vqwMGrf/Noob.webp",
	"https://i.ibb.co/09PTcf1/Khush.webp",
	"https://i.ibb.co/mysnWys/ljq8888001.webp",
	"https://i.ibb.co/j8pwLZ4/Philip-Gouda-alias.webp",
	"https://i.ibb.co/KXD02Km/Air.webp",
	"https://i.ibb.co/3pGgHYj/11planning.webp",
	"https://i.ibb.co/3FRHNRG/Josianebih.webp",
	"https://i.ibb.co/vV7Bvsn/sms.webp",
	"https://i.ibb.co/QrpYWS3/Colin.webp",
	"https://i.ibb.co/T25ppGB/10-minute-bitcoin-fee-bot.webp",
	"https://i.ibb.co/xKvd726/SandRock.webp",
	"https://i.ibb.co/FKGRZpZ/image.webp",
	"https://i.ibb.co/xYj14wF/AUD-Exchange-Rate-Reacts-to-US-Inflation-Data-Australian-Unemployment-Forecast-and-PMIs.webp",
	"https://i.ibb.co/nLYBkCN/drikkes.webp",
	"https://i.ibb.co/VCc7Bng/Asian-Stocks-Rise-Despite-Concerns-Over-US-Economy.webp",
	"https://i.ibb.co/n6hz8nT/Lelouch-VI-Britania.webp",
	"https://i.ibb.co/LZrHc8y/5-minute-bitcoin-fee-bot.webp",
	"https://i.ibb.co/3SVpZTj/Varian-Thode.webp",
	"https://i.ibb.co/wSWz2kN/Discovering-America-s-Hidden-Gems-Where-to-Mine-Diamonds-and-Gemstones.webp",
	"https://i.ibb.co/drf8h9B/China-s-Reevaluation-of-Taiwan-Seizure-Plans-After-Iran-s-Failed-Attack-on-Israel.webp",
	"https://i.ibb.co/TBL6xn6/ASML-s-Disappointing-Results-Cause-Tech-Stocks-to-Slump-in-Europe.webp",
	"https://i.ibb.co/pyQBzgN/dfs.webp",
	"https://i.ibb.co/zZJrqnX/Demaris-Sindoni.webp",
	"https://i.ibb.co/X344Tvf/Lady-Gaga.webp",
	"https://i.ibb.co/HPg2zcS/Szekely-Gigliotti.webp",
	"https://i.ibb.co/GHGqL84/image.webp",
	"https://i.ibb.co/n8cTZrm/Noob.webp",
	"https://i.ibb.co/CV0Pk1t/Khush.webp",
	"https://i.ibb.co/MkdRCNB/ljq8888001.webp",
	"https://i.ibb.co/FK5cvVS/Philip-Gouda-alias.webp",
	"https://i.ibb.co/2sVZ6LS/Air.webp",
	"https://i.ibb.co/C78cMhW/11planning.webp",
	"https://i.ibb.co/mRDKN01/sms.webp",
	"https://i.ibb.co/tPk149t/Josianebih.webp",
	"https://i.ibb.co/4mQ6HMY/Colin.webp",
	"https://i.ibb.co/VpBR6p2/Vlado-Stoll.webp",
	"https://i.ibb.co/Dpj0T8s/10-minute-bitcoin-fee-bot.webp",
	"https://i.ibb.co/YfBzMDc/AUD-Exchange-Rate-Reacts-to-US-Inflation-Data-Australian-Unemployment-Forecast-and-PMIs.webp",
	"https://i.ibb.co/0f2Bh1d/Asian-Stocks-Rise-Despite-Concerns-Over-US-Economy.webp",
	"https://i.ibb.co/BZQCgDZ/5-minute-bitcoin-fee-bot.webp",
	"https://i.ibb.co/8KMQBsG/Uhlir-Bloodsworth.webp",
	"https://i.ibb.co/6NK7K8H/Waichi-Hatcher.webp",
	"https://i.ibb.co/Kzdn6rW/Golub-Cafaro.webp",
	"https://i.ibb.co/FgBNXR5/Culbert-Woodby.webp",
	"https://i.ibb.co/LpCHCbW/Loper-Kuennen.webp",
	"https://i.ibb.co/zs0m9DQ/Pickerill-Merla.webp",
	"https://i.ibb.co/x89zprb/Ringley-Asche.webp",
	"https://i.ibb.co/0Qn2fMg/Marloes-Varvel.webp",
	"https://i.ibb.co/TWGRKrR/Marchione-Leonhard.webp",
	"https://i.ibb.co/0tCRzfx/Psencik-Gretal.webp",
	"https://i.ibb.co/BzdzCvN/Kading-Goerke.webp",
	"https://i.ibb.co/wwZqcK6/Womac-Poehlman.webp",
	"https://i.ibb.co/Q8XR3zh/Balash-Badeaux.webp",
	"https://i.ibb.co/NprRrp0/Steinfeld-Katerina.webp",
	"https://i.ibb.co/txJLnCx/Pizarro-Bumpas.webp",
	"https://i.ibb.co/qjTL6Zn/Huerta-Aimil.webp",
	"https://i.ibb.co/v1wRxLv/Lienhard-Scotten.webp",
	"https://i.ibb.co/g6sBxS2/Majure-Rickson.webp",
	"https://i.ibb.co/t8hqP9G/Carringer-Trude.webp",
	"https://i.ibb.co/dWY0L0Y/Bruck-Billeter.webp",
	"https://i.ibb.co/Gd037zy/Precht-Hughette.webp",
	"https://i.ibb.co/s5wxjvj/Malczewski-Boecker.webp",
	"https://i.ibb.co/2y4BpZH/Peckenpaugh-Heddi.webp",
	"https://i.ibb.co/qkSD52B/Tay-Gleave.webp",
	"https://i.ibb.co/FXVgS14/Deitz-Crotteau.webp",
	"https://i.ibb.co/VCFz0H0/Vin-Hazelton.webp",
	"https://i.ibb.co/mv1sM62/Liberato-Sistare.webp",
	"https://i.ibb.co/HzZGLdF/Parrot-Saindon.webp",
	"https://i.ibb.co/ydjncGG/Jessup-Ibarra.webp",
	"https://i.ibb.co/GWYVb2H/Vanosdol-Blau.webp",
	"https://i.ibb.co/jDHnTKc/Fujimoto-Forsell.webp",
	"https://i.ibb.co/jTsT5dn/Lupa-Saurer.webp",
	"https://i.ibb.co/VpzzMmD/Bustard-Poquette.webp",
	"https://i.ibb.co/59K14M0/Hink-Crago.webp",
	"https://i.ibb.co/QvCdGgg/Pawlak-Dobbs.webp",
	"https://i.ibb.co/s6RQPCH/Sewall-Kokoszka.webp",
	"https://i.ibb.co/v17yrNY/Serino-Frels.webp",
	"https://i.ibb.co/7YYyfnW/Semple-Frisina.webp",
	"https://i.ibb.co/R3Znjf1/Rumpel-Stefanic.webp",
	"https://i.ibb.co/Lhr0TR5/Radford-Simmons.webp",
	"https://i.ibb.co/nnThhjL/Wind-Pinkos.webp",
	"https://i.ibb.co/5GcR8vs/Heard-Halperin.webp",
	"https://i.ibb.co/y5tWmvY/Reeds-Porth.webp",
	"https://i.ibb.co/qFNT9bc/Roode-Muckenfuss.webp",
	"https://i.ibb.co/tXJ0Z3K/Koehn-Hertlein.webp",
	"https://i.ibb.co/y4V0WMq/Cristen-Leitz.webp",
	"https://i.ibb.co/s3CKrfL/Valverde-Tribe.webp",
	"https://i.ibb.co/swYdYwT/Earll-Abrahamian.webp",
	"https://i.ibb.co/TrXX6nM/Tani-Reddoch.webp",
	"https://i.ibb.co/Qj5NpMx/Lenoir-Roseanna.webp",
	"https://i.ibb.co/NSPPRCP/Zenger-Slaten.webp",
	"https://i.ibb.co/Kw3zzY7/Ferg-Whiten.webp",
	"https://i.ibb.co/23Lp9ZY/Storck-Timmer.webp",
	"https://i.ibb.co/C1k8jbG/Chappel-Clouser.webp",
	"https://i.ibb.co/dfsShW2/Lennard-Audwin.webp",
	"https://i.ibb.co/H7DYhcR/Mavra-Rosin.webp",
	"https://i.ibb.co/sRpNdJY/Lukasik-Zenas.webp",
	"https://i.ibb.co/Yh46yHb/Petrucelli-Stallard.webp",
	"https://i.ibb.co/sCzwjBw/Shelstad-Bellinger.webp",
	"https://i.ibb.co/d4z7KMX/Salmela-Rolando.webp",
	"https://i.ibb.co/NFdFmpR/Iida-Sage.webp",
	"https://i.ibb.co/27gSnwY/Mctague-Joly.webp",
	"https://i.ibb.co/R2mXsLW/Linsky-Valasek.webp",
	"https://i.ibb.co/GQqpxxY/Mobbs-Drum.webp",
	"https://i.ibb.co/ZS2Hdp0/Rogacki-Piraino.webp",
	"https://i.ibb.co/fNfznX7/Yuhasz-Speckman.webp",
	"https://i.ibb.co/zskcnrv/Engelke-Kurilla.webp",
	"https://i.ibb.co/hWPyZHG/Rundquist-Dzik.webp",
	"https://i.ibb.co/gV53bzN/Novelia-Kazarian.webp",
	"https://i.ibb.co/vXmfTKz/Vayda-Scarpulla.webp",
	"https://i.ibb.co/jwtrX5j/Amer-Younkins.webp",
	"https://i.ibb.co/5BhGmwd/Digiacomo-Coley.webp",
	"https://i.ibb.co/qgnzxQc/Covault-Oyster.webp",
	"https://i.ibb.co/V9sm0bp/Hephzibah-Ritson.webp",
	"https://i.ibb.co/d2FwwYM/Nial-Zuchowski.webp",
	"https://i.ibb.co/VmRzrMJ/Stringham-Lerner.webp",
	"https://i.ibb.co/M6n6vdW/Nesbitt-Deschene.webp",
	"https://i.ibb.co/yWqbhGG/Osaki-Eberhardt.webp",
	"https://i.ibb.co/SQk1gdV/Linderman-Strom.webp",
}

func (f *fetcher) generateProfilePhoto(ctx context.Context, name string) (string, error) {
	if f.uploadKey == "" {
		idx := atomic.AddUint64(&f.imgsLBIndex, 1) % uint64(len(images))
		return images[idx], nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://thispersondoesnotexist.com/", nil)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate random photo")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate random photo")
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("status %v on getting random photo", resp.StatusCode)
	}
	defer resp.Body.Close()
	jpeg, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read random photo")
	}
	im, err := vips.NewImageFromBuffer(jpeg)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load random photo")
	}
	webpData, _, err := im.ExportWebp(&vips.WebpExportParams{
		StripMetadata: true,
		Quality:       80,
		Lossless:      false,
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to convert random photo to webp")
	}
	img, _ := imgbb.NewImageFromFile(name, 3*30*24*60, webpData)
	uplResp, err := f.webpUploadClient.Upload(ctx, img)
	if err != nil {
		return "", errors.Wrapf(err, "failed to upload image")
	}
	if uplResp.Success {
		return uplResp.Data.URL, nil
	} else {
		return "", errors.Errorf("failed to upload image: %v", uplResp.StatusCode)
	}
}
