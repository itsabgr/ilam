git remote add target https://abgr:Bn9zK6ASFTx9RYAgwCci@git.baftpaydarshahr.ir/abgr/ilam.git
case "push" in
    push)
        GIT_SSL_NO_VERIFY=true git push -f --all target
        GIT_SSL_NO_VERIFY=true git push -f --tags target
        ;;
    delete)
        GIT_SSL_NO_VERIFY=true git push -d target ${GITHUB_EVENT_REF}
        ;;
    *)
        break
        ;;
esac
git remote remove target